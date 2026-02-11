// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"fmt"
	"strconv"
	"sync"
)

type SemanticContextID uint32

const (
	NoneSemanticContextID SemanticContextID = 0
	SemanticContextTypeEmpty  int8          = 1
	SemanticContextTypePred   int8          = 2
	SemanticContextTypePreced int8          = 3
	SemanticContextTypeAND    int8          = 4
	SemanticContextTypeOR     int8          = 5
)

type SemanticContextData struct {
	scType      int8
	ruleIndex   int32
	predIndex   int32
	isDependent bool
	opOffset    uint32
	opCount     uint32
}

var (
	nextSemanticIDValue SemanticContextID
	semanticRegistry    []SemanticContextData
	semanticOperands    []SemanticContextID
	semanticLock        sync.RWMutex
	semanticBridgeCache [65536]SemanticContext
	SemanticContextNone SemanticContext
)

func init() {
	nextSemanticIDValue = 1
	semanticRegistry = make([]SemanticContextData, 1, 1000)
	semanticOperands = make([]SemanticContextID, 0, 1000)
	SemanticContextNone = NewPredicate(-1, -1, false)
	nextSemanticID(SemanticContextNone)
}

func nextSemanticID(s SemanticContext) SemanticContextID {
	if s == nil {
		return NoneSemanticContextID
	}
	semanticLock.Lock()
	defer semanticLock.Unlock()
	return NextSemanticIDInternal(s)
}

func NextSemanticIDInternal(s SemanticContext) SemanticContextID {
	if s == nil {
		return NoneSemanticContextID
	}

	// Get current ID
	var currentID SemanticContextID
	switch t := s.(type) {
	case *Predicate:
		currentID = t.id
	case *PrecedencePredicate:
		currentID = t.id
	case *AND:
		currentID = t.id
	case *OR:
		currentID = t.id
	}

	if currentID != NoneSemanticContextID {
		return currentID
	}

	// Recursively assign IDs to operands
	switch t := s.(type) {
	case *AND:
		for _, op := range t.opnds {
			NextSemanticIDInternal(op)
		}
	case *OR:
		for _, op := range t.opnds {
			NextSemanticIDInternal(op)
		}
	}

	id := nextSemanticIDValue
	nextSemanticIDValue++

	data := SemanticContextData{}
	switch t := s.(type) {
	case *Predicate:
		t.id = id
		data.scType = SemanticContextTypePred
		data.ruleIndex = int32(t.ruleIndex)
		data.predIndex = int32(t.predIndex)
		data.isDependent = t.isCtxDependent
	case *PrecedencePredicate:
		t.id = id
		data.scType = SemanticContextTypePreced
		data.predIndex = int32(t.precedence)
	case *AND:
		t.id = id
		data.scType = SemanticContextTypeAND
		data.opOffset = uint32(len(semanticOperands))
		data.opCount = uint32(len(t.opnds))
		for _, op := range t.opnds {
			semanticOperands = append(semanticOperands, getSemanticID(op))
		}
	case *OR:
		t.id = id
		data.scType = SemanticContextTypeOR
		data.opOffset = uint32(len(semanticOperands))
		data.opCount = uint32(len(t.opnds))
		for _, op := range t.opnds {
			semanticOperands = append(semanticOperands, getSemanticID(op))
		}
	}

	semanticRegistry = append(semanticRegistry, data)
	return id
}

func getSemanticID(s SemanticContext) SemanticContextID {
	if s == nil {
		return NoneSemanticContextID
	}
	switch t := s.(type) {
	case *Predicate:
		return t.id
	case *PrecedencePredicate:
		return t.id
	case *AND:
		return t.id
	case *OR:
		return t.id
	}
	return NoneSemanticContextID
}

func getSemanticContextByID(id SemanticContextID) SemanticContext {
	if id == NoneSemanticContextID {
		return nil
	}
	semanticLock.RLock()
	if int(id) >= len(semanticRegistry) {
		semanticLock.RUnlock()
		return nil
	}
	semanticLock.RUnlock()

	slot := id % 65536
	cached := semanticBridgeCache[slot]
	if cached != nil && getSemanticID(cached) == id {
		return cached
	}

	semanticLock.RLock()
	data := semanticRegistry[id]
	semanticLock.RUnlock()

	var res SemanticContext
	switch data.scType {
	case SemanticContextTypePred:
		res = NewPredicate(int(data.ruleIndex), int(data.predIndex), data.isDependent)
		res.(*Predicate).id = id
	case SemanticContextTypePreced:
		res = NewPrecedencePredicate(int(data.predIndex))
		res.(*PrecedencePredicate).id = id
	case SemanticContextTypeAND:
		ops := make([]SemanticContext, data.opCount)
		for i := uint32(0); i < data.opCount; i++ {
			ops[i] = getSemanticContextByID(semanticOperands[data.opOffset+i])
		}
		res = &AND{id: id, opnds: ops}
	case SemanticContextTypeOR:
		ops := make([]SemanticContext, data.opCount)
		for i := uint32(0); i < data.opCount; i++ {
			ops[i] = getSemanticContextByID(semanticOperands[data.opOffset+i])
		}
		res = &OR{id: id, opnds: ops}
	}

	semanticBridgeCache[slot] = res
	return res
}

// SemanticContext is a tree structure used to record the semantic context in which
//
//	an ATN configuration is valid.  It's either a single predicate,
//	a conjunction p1 && p2, or a sum of products p1 || p2.
//
//	I have scoped the AND, OR, and Predicate subclasses of
//	[SemanticContext] within the scope of this outer ``class''
type SemanticContext interface {
	Equals(other Collectable[SemanticContext]) bool
	Hash() int

	evaluate(parser Recognizer, outerContext RuleContext) bool
	evalPrecedence(parser Recognizer, outerContext RuleContext) SemanticContext

	String() string
}

func SemanticContextandContext(a, b SemanticContext) SemanticContext {
	if a == nil || a == SemanticContextNone {
		return b
	}
	if b == nil || b == SemanticContextNone {
		return a
	}
	result := NewAND(a, b)
	if len(result.opnds) == 1 {
		return result.opnds[0]
	}

	return result
}

func SemanticContextorContext(a, b SemanticContext) SemanticContext {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a == SemanticContextNone || b == SemanticContextNone {
		return SemanticContextNone
	}
	result := NewOR(a, b)
	if len(result.opnds) == 1 {
		return result.opnds[0]
	}

	return result
}

type Predicate struct {
	id             SemanticContextID
	ruleIndex      int
	predIndex      int
	isCtxDependent bool
}

func NewPredicate(ruleIndex, predIndex int, isCtxDependent bool) *Predicate {
	p := new(Predicate)

	p.ruleIndex = ruleIndex
	p.predIndex = predIndex
	p.isCtxDependent = isCtxDependent // e.g., $i ref in pred
	return p
}

//The default {@link SemanticContext}, which is semantically equivalent to
//a predicate of the form {@code {true}?}.

func (p *Predicate) evalPrecedence(_ Recognizer, _ RuleContext) SemanticContext {
	return p
}

func (p *Predicate) evaluate(parser Recognizer, outerContext RuleContext) bool {

	var localctx RuleContext

	if p.isCtxDependent {
		localctx = outerContext
	}

	return parser.Sempred(localctx, p.ruleIndex, p.predIndex)
}

func (p *Predicate) Equals(other Collectable[SemanticContext]) bool {
	if p == other {
		return true
	}
	otherP, ok := other.(*Predicate)
	if !ok {
		return false
	}
	if p.id != NoneSemanticContextID && otherP.id != NoneSemanticContextID {
		return p.id == otherP.id
	}
	return p.ruleIndex == otherP.ruleIndex &&
		p.predIndex == otherP.predIndex &&
		p.isCtxDependent == otherP.isCtxDependent
}

func (p *Predicate) Hash() int {
	if p.id != NoneSemanticContextID {
		return int(p.id)
	}
	h := murmurInit(0)
	h = murmurUpdate(h, p.ruleIndex)
	h = murmurUpdate(h, p.predIndex)
	if p.isCtxDependent {
		h = murmurUpdate(h, 1)
	} else {
		h = murmurUpdate(h, 0)
	}
	return murmurFinish(h, 3)
}

func (p *Predicate) String() string {
	return "{" + strconv.Itoa(p.ruleIndex) + ":" + strconv.Itoa(p.predIndex) + "}?"
}

type PrecedencePredicate struct {
	id         SemanticContextID
	precedence int
}

func NewPrecedencePredicate(precedence int) *PrecedencePredicate {

	p := new(PrecedencePredicate)
	p.precedence = precedence

	return p
}

func (p *PrecedencePredicate) evaluate(parser Recognizer, outerContext RuleContext) bool {
	return parser.Precpred(outerContext, p.precedence)
}

func (p *PrecedencePredicate) evalPrecedence(parser Recognizer, outerContext RuleContext) SemanticContext {
	if parser.Precpred(outerContext, p.precedence) {
		return SemanticContextNone
	}

	return nil
}

func (p *PrecedencePredicate) compareTo(other *PrecedencePredicate) int {
	return p.precedence - other.precedence
}

func (p *PrecedencePredicate) Equals(other Collectable[SemanticContext]) bool {

	var op *PrecedencePredicate
	var ok bool
	if op, ok = other.(*PrecedencePredicate); !ok {
		return false
	}

	if p == op {
		return true
	}

	if p.id != NoneSemanticContextID && op.id != NoneSemanticContextID {
		return p.id == op.id
	}

	return p.precedence == op.precedence
}

func (p *PrecedencePredicate) Hash() int {
	if p.id != NoneSemanticContextID {
		return int(p.id)
	}
	h := uint32(1)
	h = 31*h + uint32(p.precedence)
	return int(h)
}

func (p *PrecedencePredicate) String() string {
	return "{" + strconv.Itoa(p.precedence) + ">=prec}?"
}

func PrecedencePredicatefilterPrecedencePredicates(operands []SemanticContext) ([]SemanticContext, []*PrecedencePredicate) {
	var result []*PrecedencePredicate
	var filtered []SemanticContext

	for _, v := range operands {
		if c2, ok := v.(*PrecedencePredicate); ok {
			result = append(result, c2)
		} else {
			filtered = append(filtered, v)
		}
	}

	return filtered, result
}

// A semantic context which is true whenever none of the contained contexts
// is false.`

type AND struct {
	id    SemanticContextID
	opnds []SemanticContext
}

func addSemanticContext(operands []SemanticContext, op SemanticContext) []SemanticContext {
	for _, existing := range operands {
		if existing.Equals(op) {
			return operands
		}
	}
	return append(operands, op)
}

func NewAND(a, b SemanticContext) *AND {

	var operands []SemanticContext
	if aa, ok := a.(*AND); ok {
		for _, o := range aa.opnds {
			operands = addSemanticContext(operands, o)
		}
	} else {
		operands = addSemanticContext(operands, a)
	}

	if ba, ok := b.(*AND); ok {
		for _, o := range ba.opnds {
			operands = addSemanticContext(operands, o)
		}
	} else {
		operands = addSemanticContext(operands, b)
	}

	filtered, precedencePredicates := PrecedencePredicatefilterPrecedencePredicates(operands)
	if len(precedencePredicates) > 0 {
		// interested in the transition with the lowest precedence
		var reduced *PrecedencePredicate

		for _, p := range precedencePredicates {
			if reduced == nil || p.precedence < reduced.precedence {
				reduced = p
			}
		}

		operands = addSemanticContext(filtered, reduced)
	} else {
		operands = filtered
	}

	and := new(AND)
	and.opnds = operands

	return and
}

func (a *AND) Equals(other Collectable[SemanticContext]) bool {
	if a == other {
		return true
	}
	otherA, ok := other.(*AND)
	if !ok {
		return false
	}
	if a.id != NoneSemanticContextID && otherA.id != NoneSemanticContextID {
		return a.id == otherA.id
	}
	if len(a.opnds) != len(otherA.opnds) {
		return false
	}
	for i, v := range otherA.opnds {
		if !a.opnds[i].Equals(v) {
			return false
		}
	}
	return true
}

// {@inheritDoc}
//
// <p>
// The evaluation of predicates by a context is short-circuiting, but
// unordered.</p>
func (a *AND) evaluate(parser Recognizer, outerContext RuleContext) bool {
	for i := 0; i < len(a.opnds); i++ {
		if !a.opnds[i].evaluate(parser, outerContext) {
			return false
		}
	}
	return true
}

func (a *AND) evalPrecedence(parser Recognizer, outerContext RuleContext) SemanticContext {
	differs := false
	operands := make([]SemanticContext, 0)

	for i := 0; i < len(a.opnds); i++ {
		context := a.opnds[i]
		evaluated := context.evalPrecedence(parser, outerContext)
		differs = differs || (evaluated != context)
		if evaluated == nil {
			// The AND context is false if any element is false
			return nil
		} else if evaluated != SemanticContextNone {
			// Reduce the result by Skipping true elements
			operands = append(operands, evaluated)
		}
	}
	if !differs {
		return a
	}

	if len(operands) == 0 {
		// all elements were true, so the AND context is true
		return SemanticContextNone
	}

	var result SemanticContext

	for _, o := range operands {
		if result == nil {
			result = o
		} else {
			result = SemanticContextandContext(result, o)
		}
	}

	return result
}

func (a *AND) Hash() int {
	if a.id != NoneSemanticContextID {
		return int(a.id)
	}
	h := murmurInit(37) // Init with a value different from OR
	for _, op := range a.opnds {
		h = murmurUpdate(h, op.Hash())
	}
	return murmurFinish(h, len(a.opnds))
}

func (o *OR) Hash() int {
	if o.id != NoneSemanticContextID {
		return int(o.id)
	}
	h := murmurInit(41) // Init with o value different from AND
	for _, op := range o.opnds {
		h = murmurUpdate(h, op.Hash())
	}
	return murmurFinish(h, len(o.opnds))
}

func (a *AND) String() string {
	s := ""

	for _, o := range a.opnds {
		s += "&& " + fmt.Sprint(o)
	}

	if len(s) > 3 {
		return s[0:3]
	}

	return s
}

//
// A semantic context which is true whenever at least one of the contained
// contexts is true.
//

type OR struct {
	id    SemanticContextID
	opnds []SemanticContext
}

func NewOR(a, b SemanticContext) *OR {

	var operands []SemanticContext
	if aa, ok := a.(*OR); ok {
		for _, o := range aa.opnds {
			operands = addSemanticContext(operands, o)
		}
	} else {
		operands = addSemanticContext(operands, a)
	}

	if ba, ok := b.(*OR); ok {
		for _, o := range ba.opnds {
			operands = addSemanticContext(operands, o)
		}
	} else {
		operands = addSemanticContext(operands, b)
	}

	filtered, precedencePredicates := PrecedencePredicatefilterPrecedencePredicates(operands)
	if len(precedencePredicates) > 0 {
		// interested in the transition with the lowest precedence
		var reduced *PrecedencePredicate

		for _, p := range precedencePredicates {
			if reduced == nil || p.precedence > reduced.precedence {
				reduced = p
			}
		}

		operands = addSemanticContext(filtered, reduced)
	} else {
		operands = filtered
	}

	o := new(OR)
	o.opnds = operands

	return o
}

func (o *OR) Equals(other Collectable[SemanticContext]) bool {
	if o == other {
		return true
	}
	otherO, ok := other.(*OR)
	if !ok {
		return false
	}
	if o.id != NoneSemanticContextID && otherO.id != NoneSemanticContextID {
		return o.id == otherO.id
	}
	if len(o.opnds) != len(otherO.opnds) {
		return false
	}
	for i, v := range otherO.opnds {
		if !o.opnds[i].Equals(v) {
			return false
		}
	}
	return true
}

// <p>
// The evaluation of predicates by o context is short-circuiting, but
// unordered.</p>
func (o *OR) evaluate(parser Recognizer, outerContext RuleContext) bool {
	for i := 0; i < len(o.opnds); i++ {
		if o.opnds[i].evaluate(parser, outerContext) {
			return true
		}
	}
	return false
}

func (o *OR) evalPrecedence(parser Recognizer, outerContext RuleContext) SemanticContext {
	differs := false
	operands := make([]SemanticContext, 0)
	for i := 0; i < len(o.opnds); i++ {
		context := o.opnds[i]
		evaluated := context.evalPrecedence(parser, outerContext)
		differs = differs || (evaluated != context)
		if evaluated == SemanticContextNone {
			// The OR context is true if any element is true
			return SemanticContextNone
		} else if evaluated != nil {
			// Reduce the result by Skipping false elements
			operands = append(operands, evaluated)
		}
	}
	if !differs {
		return o
	}
	if len(operands) == 0 {
		// all elements were false, so the OR context is false
		return nil
	}
	var result SemanticContext

	for _, o := range operands {
		if result == nil {
			result = o
		} else {
			result = SemanticContextorContext(result, o)
		}
	}

	return result
}

func (o *OR) String() string {
	s := ""

	for _, o := range o.opnds {
		s += "|| " + fmt.Sprint(o)
	}

	if len(s) > 3 {
		return s[0:3]
	}

	return s
}
