// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"reflect"
	"strconv"
	"sync"
)

var (
	baseParserRuleContextPool = sync.Pool{
		New: func() interface{} {
			return new(BaseParserRuleContext)
		},
	}
)

// ParserRuleContextEmpty is a global singleton for an empty parser rule context.
// It's initialized carefully after types are defined.
var ParserRuleContextEmpty *BaseParserRuleContext

func init() {
	// Initialize ParserRuleContextEmpty.
	// It should not come from the pool and should be unique.
	ParserRuleContextEmpty = &BaseParserRuleContext{
		invokingState: -1,
		RuleIndex:     -1, // Explicitly set, though -1 is default for int
		// parentCtx, start, stop, exception, children will be nil by default.
	}
}

type ParserRuleContext interface {
	RuleContext // Embeds RuleContext interface

	SetException(RecognitionException)
	GetException() RecognitionException // Added getter for completeness

	AddTokenNode(token Token) *TerminalNodeImpl
	AddErrorNode(badToken Token) *ErrorNodeImpl

	EnterRule(listener ParseTreeListener)
	ExitRule(listener ParseTreeListener)

	SetStart(Token)
	GetStart() Token

	SetStop(Token)
	GetStop() Token

	AddChild(child RuleContext) RuleContext // General child addition
	RemoveLastChild()
}

type BaseParserRuleContext struct {
	parentCtx     RuleContext // Can be ParserRuleContext or other RuleContext impls
	invokingState int
	RuleIndex     int

	start, stop Token                  // Matched start and stop tokens
	exception   RecognitionException   // Exception that occurred during parsing this rule
	children    []Tree                 // List of children (tokens or other rule contexts)
}

func (p *BaseParserRuleContext) Reset() {
	p.parentCtx = nil
	p.invokingState = 0 // Default to 0, will be set by constructor logic
	p.RuleIndex = -1    // Default to -1, indicating not a specific rule index unless set

	p.start = nil
	p.stop = nil
	p.exception = nil

	// Clear children slice, retaining underlying array if possible
	// Also nil out references to help GC, especially if children are pooled.
	for i := range p.children {
		p.children[i] = nil
	}
	p.children = p.children[:0]
}

func releaseBaseParserRuleContext(prc *BaseParserRuleContext) {
	if prc == nil || prc == ParserRuleContextEmpty { // Do not pool the global empty instance or nil
		return
	}
	prc.Reset()
	baseParserRuleContextPool.Put(prc)
}

// NewBaseParserRuleContext creates a new BaseParserRuleContext, potentially from pool.
// The parent can be nil (e.g. for the start rule).
// invokingStateNumber is the state that invoked this rule.
func NewBaseParserRuleContext(parent ParserRuleContext, invokingStateNumber int) *BaseParserRuleContext {
	prc := baseParserRuleContextPool.Get().(*BaseParserRuleContext)

	// Initialize fields (was previously in InitBaseParserRuleContext)
	prc.parentCtx = parent
	if parent == nil {
		prc.invokingState = -1 // Standard for rules with no caller (like start rule)
	} else {
		prc.invokingState = invokingStateNumber
	}

	prc.RuleIndex = -1 // Default, specific rules will set this via generated code
	// Reset ensures these are nil/empty, but good to be explicit if defaults were different
	prc.children = prc.children[:0] // Ensure empty, but keep capacity
	prc.start = nil
	prc.stop = nil
	prc.exception = nil

	return prc
}

// InitBaseParserRuleContext is now integrated into NewBaseParserRuleContext.
// Kept as commented out for reference to original structure if needed.
/*
func InitBaseParserRuleContext(prc *BaseParserRuleContext, parent ParserRuleContext, invokingStateNumber int) {
	prc.parentCtx = parent
	if parent == nil {
		prc.invokingState = -1
	} else {
		prc.invokingState = invokingStateNumber
	}
	prc.RuleIndex = -1
	prc.children = nil
	prc.start = nil
	prc.stop = nil
	prc.exception = nil
}
*/

func (prc *BaseParserRuleContext) SetException(e RecognitionException) {
	prc.exception = e
}

func (prc *BaseParserRuleContext) GetException() RecognitionException {
	return prc.exception
}


func (prc *BaseParserRuleContext) GetChildren() []Tree {
	return prc.children
}

// CopyFrom copies data from another BaseParserRuleContext.
// This is typically used for alternative contexts in generated parsers.
// Note: Children are NOT copied by this method.
func (prc *BaseParserRuleContext) CopyFrom(ctx *BaseParserRuleContext) {
	prc.parentCtx = ctx.parentCtx
	prc.invokingState = ctx.invokingState
	prc.RuleIndex = ctx.RuleIndex // Copy RuleIndex as well

	prc.start = ctx.start
	prc.stop = ctx.stop
	prc.exception = ctx.exception // Copy exception status

	// Children are intentionally not copied here as per original ANTLR Java logic.
	// The new context will form its own children list.
	prc.children = prc.children[:0] // Ensure current children are cleared
}

func (prc *BaseParserRuleContext) GetText() string {
	if prc.GetChildCount() == 0 {
		return ""
	}
	// Consider using strings.Builder for efficiency if called frequently on large trees
	var s string
	for _, child := range prc.children {
		if pt, ok := child.(ParseTree); ok { // Ensure child is a ParseTree
			s += pt.GetText()
		}
	}
	return s
}

func (prc *BaseParserRuleContext) EnterRule(listener ParseTreeListener) {
	// Default implementation does nothing. Generated parsers override this.
}

func (prc *BaseParserRuleContext) ExitRule(listener ParseTreeListener) {
	// Default implementation does nothing. Generated parsers override this.
}

// addTerminalNodeChild adds a terminal node as a child.
// It's an internal helper, usually called by AddTokenNode or AddErrorNode.
func (prc *BaseParserRuleContext) addTerminalNodeChild(child TerminalNode) TerminalNode {
	if child == nil {
		panic("Child TerminalNode cannot be nil") // Or handle error gracefully
	}
	if prc.children == nil { // Should be initialized to empty slice by Reset/New
		prc.children = make([]Tree, 0, 1) // Allocate with some capacity if first child
	}
	prc.children = append(prc.children, child)
	// The child's parent is set by the caller (AddTokenNode/AddErrorNode)
	return child
}

// AddChild adds a RuleContext as a child.
// It sets the parent of the child to this context.
func (prc *BaseParserRuleContext) AddChild(child RuleContext) RuleContext {
	if child == nil {
		panic("Child RuleContext cannot be nil")
	}
	if prc.children == nil {
		prc.children = make([]Tree, 0, 1)
	}
	prc.children = append(prc.children, child)
	// Set parent link for the added child
	if pTreeChild, ok := child.(ParseTree); ok {
		pTreeChild.SetParent(prc)
	}
	return child
}

func (prc *BaseParserRuleContext) RemoveLastChild() {
	if prc.children != nil && len(prc.children) > 0 {
		// Potentially, if child was pooled, it could be released here if its lifecycle ends.
		// However, standard ANTLR behavior doesn't automatically pool/release children upon removal.
		// lastChild := prc.children[len(prc.children)-1]
		prc.children = prc.children[:len(prc.children)-1]
		// If lastChild was a pooled BaseParserRuleContext, consider:
		// if pooledChild, ok := lastChild.(*BaseParserRuleContext); ok {
		//    releaseBaseParserRuleContext(pooledChild) // Requires careful lifecycle analysis
		// }
	}
}

func (prc *BaseParserRuleContext) AddTokenNode(token Token) *TerminalNodeImpl {
	if token == nil { // Guard against nil token
		// Handle error: panic, return nil, or add a specific error node if desired
		// For now, let's assume token is valid as per original implicit expectation
	}
	// NewTerminalNodeImpl might be a candidate for pooling if created very frequently
	node := NewTerminalNodeImpl(token)
	node.SetParent(prc) // Set parent link
	prc.addTerminalNodeChild(node)
	return node
}

func (prc *BaseParserRuleContext) AddErrorNode(badToken Token) *ErrorNodeImpl {
	if badToken == nil {
		// As above, assume badToken is valid for now
	}
	// NewErrorNodeImpl might also be a candidate for pooling
	node := NewErrorNodeImpl(badToken)
	node.SetParent(prc) // Set parent link
	prc.addTerminalNodeChild(node)
	return node
}

func (prc *BaseParserRuleContext) GetChild(i int) Tree {
	if prc.children != nil && i >= 0 && i < len(prc.children) { // Added bounds check for i
		return prc.children[i]
	}
	return nil
}

// GetChildOfType retrieves a child of a specific Go type.
func (prc *BaseParserRuleContext) GetChildOfType(i int, childType reflect.Type) RuleContext {
    if childType == nil || i < 0 { return nil } // Invalid args

    count := 0
    for _, child := range prc.children {
        // Check if child's actual type matches childType directly (for concrete types)
        // or if child implements childType (for interface types).
        // reflect.TypeOf(child) gives concrete type.
        // childType is the target type (can be interface or concrete).

        // This logic needs to be careful. If childType is an interface, we need Implements.
        // If childType is a struct, we need direct type equality or AssignableTo/ConvertibleTo.
        // The original code used `reflect.TypeOf(child) == childType`. This only works for concrete types.
        // A more robust check:
        v := reflect.ValueOf(child)
        if v.IsValid() && v.Type().AssignableTo(childType) { // Check assignability
            if count == i {
                if rc, ok := child.(RuleContext); ok { // Ensure it's a RuleContext
                    return rc
                }
                return nil // Matched type but not RuleContext, should not happen if children are RuleContexts
            }
            count++
        }
    }
    return nil
}


func (prc *BaseParserRuleContext) ToStringTree(ruleNames []string, recog Recognizer) string {
	return TreesStringTree(prc, ruleNames, recog) // Delegate to utility function
}

func (prc *BaseParserRuleContext) GetRuleContext() RuleContext {
	return prc // BaseParserRuleContext is a RuleContext
}

func (prc *BaseParserRuleContext) Accept(visitor ParseTreeVisitor) interface{} {
	return visitor.VisitChildren(prc) // Default behavior for visitor pattern
}

func (prc *BaseParserRuleContext) SetStart(t Token) {
	prc.start = t
}

func (prc *BaseParserRuleContext) GetStart() Token {
	return prc.start
}

func (prc *BaseParserRuleContext) SetStop(t Token) {
	prc.stop = t
}

func (prc *BaseParserRuleContext) GetStop() Token {
	return prc.stop
}

// GetToken retrieves a specific TerminalNode child matching token type and occurrence.
func (prc *BaseParserRuleContext) GetToken(ttype int, i int) TerminalNode {
    if i < 0 || prc.children == nil { return nil } // Invalid index or no children

    occurrence := 0
    for _, child := range prc.children {
        if tn, ok := child.(TerminalNode); ok {
            if tn.GetSymbol().GetTokenType() == ttype {
                if occurrence == i {
                    return tn
                }
                occurrence++
            }
        }
    }
    return nil // Not found
}

// GetTokens retrieves all TerminalNode children matching a token type.
func (prc *BaseParserRuleContext) GetTokens(ttype int) []TerminalNode {
	if prc.children == nil {
		return make([]TerminalNode, 0) // Return empty slice, not nil
	}
	tokens := make([]TerminalNode, 0) // Initialize with 0 length
	for _, child := range prc.children {
		if tn, ok := child.(TerminalNode); ok {
			if tn.GetSymbol().GetTokenType() == ttype {
				tokens = append(tokens, tn)
			}
		}
	}
	return tokens
}

func (prc *BaseParserRuleContext) GetPayload() interface{} {
	return prc // The context itself is the payload
}

// getChild is an internal helper. Use GetTypedRuleContext for external use.
func (prc *BaseParserRuleContext) getChild(ctxType reflect.Type, i int) RuleContext {
	if prc.children == nil || i < 0 { return nil }

	occurrence := 0
	for _, childNode := range prc.children {
		// Check if childNode's type is assignable to ctxType
		// This is more flexible than direct type equality, handles interfaces.
		val := reflect.ValueOf(childNode)
		if val.IsValid() && val.Type().AssignableTo(ctxType) {
			if occurrence == i {
				if rc, ok := childNode.(RuleContext); ok { // Cast back to RuleContext
					return rc
				}
				// Should not happen if children are always RuleContexts and type check is correct
				return nil
			}
			occurrence++
		}
	}
	return nil
}

// GetTypedRuleContext retrieves a child of a specific RuleContext type (interface or concrete).
func (prc *BaseParserRuleContext) GetTypedRuleContext(ctxType reflect.Type, i int) RuleContext {
	return prc.getChild(ctxType, i)
}

// GetTypedRuleContexts retrieves all children of a specific RuleContext type.
func (prc *BaseParserRuleContext) GetTypedRuleContexts(ctxType reflect.Type) []RuleContext {
	if prc.children == nil {
		return make([]RuleContext, 0)
	}
	contexts := make([]RuleContext, 0)
	for _, childNode := range prc.children {
		val := reflect.ValueOf(childNode)
		if val.IsValid() && val.Type().AssignableTo(ctxType) {
			if rc, ok := childNode.(RuleContext); ok {
				contexts = append(contexts, rc)
			}
		}
	}
	return contexts
}

func (prc *BaseParserRuleContext) GetChildCount() int {
	if prc.children == nil {
		return 0
	}
	return len(prc.children)
}

func (prc *BaseParserRuleContext) GetSourceInterval() Interval {
	if prc.start == nil || prc.stop == nil {
		return TreeInvalidInterval // Use the defined invalid interval
	}
	// Ensure token indices are valid before creating interval
	startIndex := prc.start.GetTokenIndex()
	stopIndex := prc.stop.GetTokenIndex()
	if startIndex == -1 || stopIndex == -1 { // Check for BadToken index
	    return TreeInvalidInterval
	}
	return NewInterval(startIndex, stopIndex)
}

// String method for debugging, creating LISP-style tree representation.
func (prc *BaseParserRuleContext) String(ruleNames []string, stop RuleContext) string {
	// Consider strings.Builder for efficiency
	var p ParserRuleContext = prc
	s := "["
	first := true
	for p != nil && p != stop {
		if !first {
			s += " "
		}
		first = false

		if ruleNames == nil { // No rule names, use invoking state
			if !p.IsEmpty() { // IsEmpty checks invokingState == -1
				s += strconv.Itoa(p.GetInvokingState())
			}
		} else { // Use rule names
			ri := p.GetRuleIndex()
			var ruleName string
			if ri >= 0 && ri < len(ruleNames) {
				ruleName = ruleNames[ri]
			} else {
				ruleName = strconv.Itoa(ri) // Fallback to index if name not found
			}
			s += ruleName
		}
		// Check parent type before asserting to ParserRuleContext for IsEmpty
		parentTree := p.GetParent()
		if parentTree != nil {
		    if parentPRC, ok := parentTree.(ParserRuleContext); !(ok && parentPRC.IsEmpty()) && ruleNames == nil {
		        // Condition was: (ruleNames != nil || !p.GetParent().(ParserRuleContext).IsEmpty())
                // This seems to add space if parent exists and is not empty (when no rule names)
                // Or if rule names are provided and parent exists.
                // Simplified: always add space if there's a parent, let the loop handle termination.
                // The original logic was complex: `if p.GetParent() != nil && (ruleNames != nil || !p.GetParent().(ParserRuleContext).IsEmpty())`
                // This is for adding space between parent names in the stack.
                // The loop `for p != nil && p != stop` controls iteration.
                // The space is between context strings in the stack representation.
            }
		}

		if parentTree != nil {
			if prcParent, ok := parentTree.(ParserRuleContext); ok {
				p = prcParent
			} else {
				break // Parent is not ParserRuleContext, stop traversal
			}
		} else {
			p = nil // No more parents
		}
	}
	s += "]"
	return s
}

// SetParent establishes the parent of this parse tree node.
func (prc *BaseParserRuleContext) SetParent(v Tree) {
	if rc, ok := v.(RuleContext); ok { // Ensure parent is a RuleContext
		prc.parentCtx = rc
	} else if v == nil {
		prc.parentCtx = nil
	}
	// Else: v is a Tree but not RuleContext, what to do?
	// Original ANTLR allows any ParseTree as parent. RuleContext is a ParseTree.
	// For BaseParserRuleContext, parentCtx is RuleContext.
	// This implies v must be a RuleContext or nil.
}

func (prc *BaseParserRuleContext) GetInvokingState() int {
	return prc.invokingState
}

func (prc *BaseParserRuleContext) SetInvokingState(t int) {
	prc.invokingState = t
}

func (prc *BaseParserRuleContext) GetRuleIndex() int {
	return prc.RuleIndex
}

// RuleContext methods (some might be overridden by specific generated contexts)
func (prc *BaseParserRuleContext) GetAltNumber() int {
	return ATNInvalidAltNumber // Default, overridden by generated code if it's a decision rule context
}

func (prc *BaseParserRuleContext) SetAltNumber(altNum int) {
	// Default implementation does nothing. Generated code for rules with multiple alts will override.
}

func (prc *BaseParserRuleContext) IsEmpty() bool {
	return prc.invokingState == -1
}

func (prc *BaseParserRuleContext) GetParent() Tree {
	return prc.parentCtx // parentCtx is RuleContext, which is a Tree
}

// InterpreterRuleContext for use by the interpreter.
type InterpreterRuleContext interface {
	ParserRuleContext
	// Potentially add methods specific to interpreter's needs if any
}

type BaseInterpreterRuleContext struct {
	*BaseParserRuleContext // Embed BaseParserRuleContext
}

// NewBaseInterpreterRuleContext creates a context for the interpreter.
// It reuses NewBaseParserRuleContext which now gets from pool.
//goland:noinspection GoUnusedExportedFunction
func NewBaseInterpreterRuleContext(parent BaseInterpreterRuleContext, invokingStateNumber, ruleIndex int) *BaseInterpreterRuleContext {
	// If parent is an interface type, ensure it's nil correctly if that's the input intention
	var parentPRC ParserRuleContext
	if parent != nil { // Check if the interface itself is nil
		parentPRC = parent
	}

	bprc := NewBaseParserRuleContext(parentPRC, invokingStateNumber)
	bprc.RuleIndex = ruleIndex // Set the specific rule index

	return &BaseInterpreterRuleContext{BaseParserRuleContext: bprc}
}

[end of runtime/Go/antlr/v4/parser_rule_context.go]
