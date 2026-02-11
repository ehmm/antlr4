// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"fmt"
)

// ATNConfigSet is a specialized set of ATNConfig that tracks information
// about its elements and can combine similar configurations using a
// graph-structured stack.
type ATNConfigSet struct {
	cachedHash int

	// configLookup maps a hash to a slice of indices into the configs slice.
	// This handles collisions while allowing us to store configs as values.
	configLookup map[int][]int

	// configs is the added elements that did not match an existing key in configLookup
	configs []ATNConfig

	// TODO: These fields make me pretty uncomfortable, but it is nice to pack up
	// info together because it saves re-computation. Can we track conflicts as they
	// are added to save scanning configs later?
	conflictingAlts *BitSet

	// dipsIntoOuterContext is used by parsers and lexers. In a lexer, it indicates
	// we hit a pred while computing a closure operation. Do not make a DFA state
	// from the ATNConfigSet in this case. TODO: How is this used by parsers?
	dipsIntoOuterContext bool

	// fullCtx is whether it is part of a full context LL prediction. Used to
	// determine how to merge $. It is a wildcard with SLL, but not for an LL
	// context merge.
	fullCtx bool

	// Used in parser and lexer. In lexer, it indicates we hit a pred
	// while computing a closure operation. Don't make a DFA state from this set.
	hasSemanticContext bool

	// readOnly is whether it is read-only. Do not
	// allow any code to manipulate the set if true because DFA states will point at
	// sets and those must not change. It not, protect other fields; conflictingAlts
	// in particular, which is assigned after readOnly.
	readOnly bool

	// TODO: These fields make me pretty uncomfortable, but it is nice to pack up
	// info together because it saves re-computation. Can we track conflicts as they
	// are added to save scanning configs later?
	uniqueAlt int

	// useObjEq indicates whether to use standard ObjEq (true) or specialized ATNConfig comparator (false)
	useObjEq bool
}

// Alts returns the combined set of alts for all the configurations in this set.
func (b *ATNConfigSet) Alts() *BitSet {
	alts := NewBitSet()
	for i := 0; i < len(b.configs); i++ {
		alts.add(b.configs[i].GetAlt())
	}
	return alts
}

// NewATNConfigSet creates a new ATNConfigSet instance.
func NewATNConfigSet(fullCtx bool) *ATNConfigSet {
	return &ATNConfigSet{
		cachedHash:   -1,
		configLookup: make(map[int][]int),
		fullCtx:      fullCtx,
		useObjEq:     false,
	}
}

func (b *ATNConfigSet) configEquals(c1, c2 *ATNConfig) bool {
	if b.useObjEq {
		return c1.Equals(c2)
	}
	return c1.PEquals(c2)
}

func (b *ATNConfigSet) configHash(c *ATNConfig) int {
	if b.useObjEq {
		return c.Hash()
	}
	return c.PHash()
}

// Add merges contexts with existing configs for (s, i, pi, _),
// where 's' is the ATNConfig.state, 'i' is the ATNConfig.alt, and
// 'pi' is the [ATNConfig].semanticContext.
//
// We use (s,i,pi) as the key.
// Updates dipsIntoOuterContext and hasSemanticContext when necessary.
func (b *ATNConfigSet) Add(config *ATNConfig, mergeCache *MergeCache) bool {
	if b.readOnly {
		panic("set is read-only")
	}

	if config.GetSemanticContext() != SemanticContextNone {
		b.hasSemanticContext = true
	}

	if config.GetReachesIntoOuterContext() > 0 {
		b.dipsIntoOuterContext = true
	}

	h := b.configHash(config)
	indices := b.configLookup[h]
	var existing *ATNConfig
	for _, idx := range indices {
		if b.configEquals(&b.configs[idx], config) {
			existing = &b.configs[idx]
			break
		}
	}

	// The config was not already in the set
	//
	if existing == nil {
		b.cachedHash = -1
		b.configs = append(b.configs, *config) // Track order here
		b.configLookup[h] = append(indices, len(b.configs)-1)
		return true
	}

	// Merge a previous (s, i, pi, _) with it and save the result
	rootIsWildcard := !b.fullCtx
	merged := merge(existing.GetContext(), config.GetContext(), rootIsWildcard, mergeCache)

	// No need to check for existing.context because config.context is in the cache,
	// since the only way to create new graphs is the "call rule" and here. We cache
	// at both places.
	existing.SetReachesIntoOuterContext(intMax(existing.GetReachesIntoOuterContext(), config.GetReachesIntoOuterContext()))

	// Preserve the precedence filter suppression during the merge
	if config.getPrecedenceFilterSuppressed() {
		existing.setPrecedenceFilterSuppressed(true)
	}

	// Replace the context because there is no need to do alt mapping
	existing.SetContext(merged)

	return true
}

// GetStates returns the set of states represented by all configurations in this config set
func (b *ATNConfigSet) GetStates() []ATNState {
	statesMap := make(map[int]ATNState)

	for i := 0; i < len(b.configs); i++ {
		state := b.configs[i].GetState()
		statesMap[state.GetStateNumber()] = state
	}

	var states []ATNState
	for _, v := range statesMap {
		states = append(states, v)
	}

	return states
}

func (b *ATNConfigSet) GetPredicates() []SemanticContext {
	predicates := make([]SemanticContext, 0)

	for i := 0; i < len(b.configs); i++ {
		c := b.configs[i].GetSemanticContext()

		if c != SemanticContextNone {
			predicates = append(predicates, c)
		}
	}

	return predicates
}

func (b *ATNConfigSet) OptimizeConfigs(interpreter *BaseATNSimulator) {
	if b.readOnly {
		panic("set is read-only")
	}

	// Empty indicate no optimization is possible
	if len(b.configLookup) == 0 {
		return
	}

	for i := 0; i < len(b.configs); i++ {
		config := &b.configs[i]
		config.SetContext(interpreter.getCachedContext(config.GetContext()))
	}
}

func (b *ATNConfigSet) AddAll(coll []*ATNConfig) bool {
	for i := 0; i < len(coll); i++ {
		b.Add(coll[i], nil)
	}

	return false
}

// Compare The configs are only equal if they are in the same order and their Equals function returns true.
// Java uses ArrayList.equals(), which requires the same order.
func (b *ATNConfigSet) Compare(bs *ATNConfigSet) bool {
	if len(b.configs) != len(bs.configs) {
		return false
	}
	for i := 0; i < len(b.configs); i++ {
		if !b.configs[i].Equals(&bs.configs[i]) {
			return false
		}
	}

	return true
}

func (b *ATNConfigSet) Equals(other Collectable[ATNConfig]) bool {
	if b == other {
		return true
	} else if _, ok := other.(*ATNConfigSet); !ok {
		return false
	}

	other2 := other.(*ATNConfigSet)
	var eca bool
	switch {
	case b.conflictingAlts == nil && other2.conflictingAlts == nil:
		eca = true
	case b.conflictingAlts != nil && other2.conflictingAlts != nil:
		eca = b.conflictingAlts.equals(other2.conflictingAlts)
	}
	return b.configs != nil &&
		b.fullCtx == other2.fullCtx &&
		b.uniqueAlt == other2.uniqueAlt &&
		eca &&
		b.hasSemanticContext == other2.hasSemanticContext &&
		b.dipsIntoOuterContext == other2.dipsIntoOuterContext &&
		b.Compare(other2)
}

func (b *ATNConfigSet) Hash() int {
	if b.readOnly {
		if b.cachedHash == -1 {
			b.cachedHash = b.hashCodeConfigs()
		}

		return b.cachedHash
	}

	return b.hashCodeConfigs()
}

func (b *ATNConfigSet) hashCodeConfigs() int {
	h := 1
	for _, config := range b.configs {
		h = 31*h + config.Hash()
	}
	return h
}

func (b *ATNConfigSet) Contains(item *ATNConfig) bool {
	if b.readOnly {
		panic("not implemented for read-only sets")
	}
	if len(b.configLookup) == 0 {
		return false
	}
	h := b.configHash(item)
	indices := b.configLookup[h]
	for _, idx := range indices {
		if b.configEquals(&b.configs[idx], item) {
			return true
		}
	}
	return false
}

func (b *ATNConfigSet) ContainsFast(item *ATNConfig) bool {
	return b.Contains(item)
}

func (b *ATNConfigSet) Clear() {
	if b.readOnly {
		panic("set is read-only")
	}
	b.configs = b.configs[:0]
	b.cachedHash = -1
	for k := range b.configLookup {
		delete(b.configLookup, k)
	}
	b.dipsIntoOuterContext = false
	b.hasSemanticContext = false
	b.uniqueAlt = 0
	if b.conflictingAlts != nil {
		b.conflictingAlts.ClearAll()
	}
}

func (b *ATNConfigSet) Commit() {
	contextLock.Lock()
	defer contextLock.Unlock()
	semanticLock.Lock()
	defer semanticLock.Unlock()
	for i := 0; i < len(b.configs); i++ {
		c := &b.configs[i]
		if c.contextID == NoneContextID && c.context != nil {
			c.contextID = nextIDInternal(c.context)
			c.context = nil
		}
		if c.semanticContextID == NoneSemanticContextID && c.semanticContext != nil {
			c.semanticContextID = NextSemanticIDInternal(c.semanticContext)
			c.semanticContext = nil
		}
	}
}

func (b *ATNConfigSet) String() string {

	s := "["

	for i, c := range b.configs {
		s += c.String()

		if i != len(b.configs)-1 {
			s += ", "
		}
	}

	s += "]"

	if b.hasSemanticContext {
		s += ",hasSemanticContext=" + fmt.Sprint(b.hasSemanticContext)
	}

	if b.uniqueAlt != ATNInvalidAltNumber {
		s += ",uniqueAlt=" + fmt.Sprint(b.uniqueAlt)
	}

	if b.conflictingAlts != nil {
		s += ",conflictingAlts=" + b.conflictingAlts.String()
	}

	if b.dipsIntoOuterContext {
		s += ",dipsIntoOuterContext"
	}

	return s
}

// NewOrderedATNConfigSet creates a config set with a slightly different Hash/Equal pair
// for use in lexers.
func NewOrderedATNConfigSet() *ATNConfigSet {
	return &ATNConfigSet{
		cachedHash:   -1,
		configLookup: make(map[int][]int),
		fullCtx:      false,
		useObjEq:     true,
	}
}
