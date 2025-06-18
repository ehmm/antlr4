// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"fmt"
	"sync"
)

const (
	lexerConfig  = iota // Indicates that this ATNConfig is for a lexer
	parserConfig        // Indicates that this ATNConfig is for a parser
)

var (
	atnConfigPool = sync.Pool{
		New: func() interface{} {
			return new(ATNConfig)
		},
	}
)

// ATNConfig is a tuple: (ATN state, predicted alt, syntactic, semantic
// context). The syntactic context is a graph-structured stack node whose
// path(s) to the root is the rule invocation(s) chain used to arrive in the
// state. The semantic context is the tree of semantic predicates encountered
// before reaching an ATN state.
type ATNConfig struct {
	precedenceFilterSuppressed     bool
	state                          ATNState
	alt                            int
	context                        *PredictionContext // Might be pooled, manage lifecycle carefully
	semanticContext                SemanticContext    // Typically singletons like SemanticContextNone
	reachesIntoOuterContext        int
	cType                          int // lexerConfig or parserConfig
	lexerActionExecutor            *LexerActionExecutor // May need pooling if complex/frequent
	passedThroughNonGreedyDecision bool
}

func (a *ATNConfig) Reset() {
	a.precedenceFilterSuppressed = false
	a.state = nil
	a.alt = 0
	// Important: If PredictionContext objects are pooled, releasing an ATNConfig
	// should not necessarily release its 'context'. The lifecycle of 'context'
	// is managed by PredictionContextCache and its users. ATNConfig just holds a reference.
	// So, we just nil it out here.
	a.context = nil // Nilling out the reference is important.
	a.semanticContext = nil // Often SemanticContextNone, which is fine. Reset to nil.
	a.reachesIntoOuterContext = 0
	a.cType = 0 // Default to 0, constructors will set specific type.
	a.lexerActionExecutor = nil
	a.passedThroughNonGreedyDecision = false
}

func releaseATNConfig(cfg *ATNConfig) {
	if cfg == nil {
		return
	}
	cfg.Reset()
	atnConfigPool.Put(cfg)
}

// NewATNConfig6 creates a new ATNConfig instance given a state, alt and context only
func NewATNConfig6(state ATNState, alt int, context *PredictionContext) *ATNConfig {
	// This is a convenience constructor, calls the more general NewATNConfig5.
	return NewATNConfig5(state, alt, context, SemanticContextNone)
}

// NewATNConfig5 creates a new ATNConfig instance given a state, alt, context and semantic context
func NewATNConfig5(state ATNState, alt int, context *PredictionContext, semanticContext SemanticContext) *ATNConfig {
	if semanticContext == nil {
		panic("semanticContext cannot be nil")
	}

	pac := atnConfigPool.Get().(*ATNConfig)
	pac.state = state
	pac.alt = alt
	pac.context = context
	pac.semanticContext = semanticContext
	pac.cType = parserConfig // This constructor is for parser configs

	// Ensure other fields are at their default/reset state from the pool
	pac.precedenceFilterSuppressed = false
	pac.reachesIntoOuterContext = 0
	pac.lexerActionExecutor = nil
	pac.passedThroughNonGreedyDecision = false
	return pac
}

// NewATNConfig4 creates a new ATNConfig instance given an existing config 'c', and a new 'state'.
// It preserves other properties from 'c'.
func NewATNConfig4(c *ATNConfig, state ATNState) *ATNConfig {
	return NewATNConfig(c, state, c.GetContext(), c.GetSemanticContext())
}

// NewATNConfig3 creates a new ATNConfig instance from 'c', with new 'state' and 'semanticContext'.
func NewATNConfig3(c *ATNConfig, state ATNState, semanticContext SemanticContext) *ATNConfig {
	return NewATNConfig(c, state, c.GetContext(), semanticContext)
}

// NewATNConfig2 creates a new ATNConfig instance from 'c', with new 'semanticContext'.
func NewATNConfig2(c *ATNConfig, semanticContext SemanticContext) *ATNConfig {
	return NewATNConfig(c, c.GetState(), c.GetContext(), semanticContext)
}

// NewATNConfig1 creates a new ATNConfig instance from 'c', with new 'state' and 'context'.
func NewATNConfig1(c *ATNConfig, state ATNState, context *PredictionContext) *ATNConfig {
	return NewATNConfig(c, state, context, c.GetSemanticContext())
}

// NewATNConfig is the most general constructor for creating a new config based on an existing one 'c',
// but with potentially overridden state, context, and semanticContext.
// It defaults to parserConfig type.
func NewATNConfig(c *ATNConfig, state ATNState, context *PredictionContext, semanticContext SemanticContext) *ATNConfig {
	b := atnConfigPool.Get().(*ATNConfig)

	// Initialize from existing config 'c'
	b.alt = c.GetAlt()
	b.reachesIntoOuterContext = c.GetReachesIntoOuterContext()
	b.precedenceFilterSuppressed = c.getPrecedenceFilterSuppressed()
	// Lexer specific fields from 'c', will be nil/false if 'c' was a parser config.
	// If 'c' could be a lexer config, these need to be copied.
	// Assuming 'c' is compatible or these are handled by cType specific logic later if needed.
	b.lexerActionExecutor = c.lexerActionExecutor
	b.passedThroughNonGreedyDecision = c.passedThroughNonGreedyDecision

	// Apply new values
	b.state = state
	b.context = context
	b.semanticContext = semanticContext

	b.cType = parserConfig // This general version is for parser configs
	return b
}

// InitATNConfig was previously used to set fields. This logic is now integrated into constructors.
// Removing to avoid confusion, as pooled objects are initialized directly by constructors.
/*
func (a *ATNConfig) InitATNConfig(c *ATNConfig, state ATNState, alt int, context *PredictionContext, semanticContext SemanticContext) {
	a.state = state
	a.alt = alt
	a.context = context
	a.semanticContext = semanticContext
	a.reachesIntoOuterContext = c.GetReachesIntoOuterContext()
	a.precedenceFilterSuppressed = c.getPrecedenceFilterSuppressed()
}
*/

func (a *ATNConfig) getPrecedenceFilterSuppressed() bool {
	return a.precedenceFilterSuppressed
}

func (a *ATNConfig) setPrecedenceFilterSuppressed(v bool) {
	a.precedenceFilterSuppressed = v
}

func (a *ATNConfig) GetState() ATNState {
	return a.state
}

func (a *ATNConfig) GetAlt() int {
	return a.alt
}

func (a *ATNConfig) SetContext(v *PredictionContext) {
	a.context = v
}

func (a *ATNConfig) GetContext() *PredictionContext {
	return a.context
}

func (a *ATNConfig) GetSemanticContext() SemanticContext {
	return a.semanticContext
}

func (a *ATNConfig) GetReachesIntoOuterContext() int {
	return a.reachesIntoOuterContext
}

func (a *ATNConfig) SetReachesIntoOuterContext(v int) {
	a.reachesIntoOuterContext = v
}

func (a *ATNConfig) Equals(o Collectable[*ATNConfig]) bool {
	if a == o { return true } // Pointer equality
	other, ok := o.(*ATNConfig)
	if !ok || other == nil { return false }

	if a.cType != other.cType { return false } // Must be same type (lexer or parser)

	switch a.cType {
	case lexerConfig:
		return a.lEquals(other) // Call private method for actual comparison
	case parserConfig:
		return a.pEquals(other) // Call private method
	default:
		// This case should ideally not be reached if cType is always valid.
		// If it can be an uninitialized or invalid cType, this might panic or behave unexpectedly.
		// Consider returning false or panicking more explicitly if cType is out of expected range.
		// For safety, if cType is unknown, they are not equal.
		return false
	}
}

// pEquals performs the comparison for parser ATNConfigs.
// Assumes 'a' and 'other' are non-nil, and both have cType == parserConfig.
func (a *ATNConfig) pEquals(other *ATNConfig) bool {
	var contextEquals bool
	if a.context == nil {
		contextEquals = (other.context == nil)
	} else {
		contextEquals = a.context.Equals(other.context) // other.context can be nil
	}

	// State can be nil in some transient ATN configurations, handle safely.
	var stateEquals bool
	if a.state == nil {
	    stateEquals = (other.state == nil)
    } else if other.state == nil {
        stateEquals = false
    } else {
        stateEquals = (a.state.GetStateNumber() == other.state.GetStateNumber())
    }

    var semContextEquals bool
    if a.semanticContext == nil {
        semContextEquals = (other.semanticContext == nil)
    } else {
        semContextEquals = a.semanticContext.Equals(other.semanticContext)
    }

	return stateEquals &&
		a.alt == other.alt &&
		semContextEquals &&
		a.precedenceFilterSuppressed == other.precedenceFilterSuppressed &&
		contextEquals
}


// PEquals is the default comparison function for a Parser ATNConfig when no specialist implementation is required
// for a collection.
// This is the exported version that was originally present.
func (a *ATNConfig) PEquals(o Collectable[*ATNConfig]) bool {
	other, ok := o.(*ATNConfig)
	if !ok || other == nil { return false }
    if a == other { return true }
    if a.cType != parserConfig || other.cType != parserConfig { return false } // Both must be parser configs
    return a.pEquals(other)
}


func (a *ATNConfig) Hash() int {
	// Hash calculation must be consistent with Equals.
	// It should also ideally produce different hashes for lexer vs parser configs if they can collide.
	// However, they are usually in different sets.
	switch a.cType {
	case lexerConfig:
		return a.LHash()
	case parserConfig:
		return a.PHash()
	default:
		// Undefined cType, return a default hash.
		// This indicates a potential issue with ATNConfig initialization.
		return -1 // Or some other sentinel hash value
	}
}

func (a *ATNConfig) PHash() int {
	contextHash := 0
	if a.context != nil { contextHash = a.context.Hash() }

	semanticContextHash := 0
	if a.semanticContext != nil { semanticContextHash = a.semanticContext.Hash() }

	stateNumber := 0
	if a.state != nil { stateNumber = a.state.GetStateNumber() }

	h := murmurInit(7) // Seed
	h = murmurUpdate(h, stateNumber)
	h = murmurUpdate(h, a.alt)
	h = murmurUpdate(h, contextHash)
	h = murmurUpdate(h, semanticContextHash)
	// For consistency with pEquals, precedenceFilterSuppressed should be in hash.
	// Original PHash had 4 items. If add this, becomes 5.
	// Let's add it for correctness. This might change behavior if existing code relies on old hash.
	if a.precedenceFilterSuppressed {
		h = murmurUpdate(h, 1)
	} else {
		h = murmurUpdate(h, 0)
	}
	return murmurFinish(h, 5) // Now 5 items hashed
}

func (a *ATNConfig) String() string {
	contextStr := "nil"
	if a.context != nil { contextStr = fmt.Sprint(a.context) }

	semanticContextStr := ""
	if a.semanticContext != SemanticContextNone && a.semanticContext != nil {
		semanticContextStr = "," + fmt.Sprint(a.semanticContext)
	} else if a.semanticContext == nil {
	    semanticContextStr = ",nil"
    }


	reachesStr := ""
	if a.reachesIntoOuterContext > 0 {
		reachesStr = ",up=" + fmt.Sprint(a.reachesIntoOuterContext)
	}

	stateStr := "nil"
	if a.state != nil { stateStr = fmt.Sprint(a.state) }

	return fmt.Sprintf("(%s,%d,[%s]%s%s)", stateStr, a.alt, contextStr, semanticContextStr, reachesStr)
}

// Lexer ATNConfig constructors
func NewLexerATNConfig6(state ATNState, alt int, context *PredictionContext) *ATNConfig {
	lac := atnConfigPool.Get().(*ATNConfig)
	lac.state = state
	lac.alt = alt
	lac.context = context // Lexers often use PredictionContext.EMPTY or nil
	lac.semanticContext = SemanticContextNone
	lac.cType = lexerConfig
	// Ensure other fields are reset
	lac.precedenceFilterSuppressed = false
	lac.reachesIntoOuterContext = 0
	lac.lexerActionExecutor = nil // Not set by this simple constructor
	lac.passedThroughNonGreedyDecision = false // Default for new lexer config
	return lac
}

func NewLexerATNConfig4(c *ATNConfig, state ATNState) *ATNConfig {
	lac := atnConfigPool.Get().(*ATNConfig)
	// Base new config on 'c', then override.
	lac.alt = c.GetAlt()
	lac.context = c.GetContext()
	lac.semanticContext = c.GetSemanticContext()
	lac.reachesIntoOuterContext = c.GetReachesIntoOuterContext() // Typically 0 for lexers
	lac.precedenceFilterSuppressed = c.getPrecedenceFilterSuppressed() // Typically false for lexers
	lac.lexerActionExecutor = c.lexerActionExecutor // Crucial for lexers

	// Apply specific changes for this constructor
	lac.state = state
	lac.passedThroughNonGreedyDecision = checkNonGreedyDecision(c, state)
	lac.cType = lexerConfig
	return lac
}

func NewLexerATNConfig3(c *ATNConfig, state ATNState, lexerActionExecutor *LexerActionExecutor) *ATNConfig {
	lac := atnConfigPool.Get().(*ATNConfig)
	// Base on 'c'
	lac.alt = c.GetAlt()
	lac.context = c.GetContext()
	lac.semanticContext = c.GetSemanticContext()
	lac.reachesIntoOuterContext = c.GetReachesIntoOuterContext()
	lac.precedenceFilterSuppressed = c.getPrecedenceFilterSuppressed()
	// No, don't copy c.lexerActionExecutor, this constructor provides a new one.
	// lac.lexerActionExecutor = c.lexerActionExecutor;

	// Apply specific changes
	lac.state = state
	lac.lexerActionExecutor = lexerActionExecutor // Set the new one
	lac.passedThroughNonGreedyDecision = checkNonGreedyDecision(c, state)
	lac.cType = lexerConfig
	return lac
}

func NewLexerATNConfig2(c *ATNConfig, state ATNState, context *PredictionContext) *ATNConfig {
	lac := atnConfigPool.Get().(*ATNConfig)
	// Base on 'c'
	lac.alt = c.GetAlt()
	// lac.context = c.GetContext(); // No, new context is provided
	lac.semanticContext = c.GetSemanticContext()
	lac.reachesIntoOuterContext = c.GetReachesIntoOuterContext()
	lac.precedenceFilterSuppressed = c.getPrecedenceFilterSuppressed()
	lac.lexerActionExecutor = c.lexerActionExecutor

	// Apply specific changes
	lac.state = state
	lac.context = context // Set the new context
	lac.passedThroughNonGreedyDecision = checkNonGreedyDecision(c, state)
	lac.cType = lexerConfig
	return lac
}

//goland:noinspection GoUnusedExportedFunction
func NewLexerATNConfig1(state ATNState, alt int, context *PredictionContext) *ATNConfig {
	// This is functionally identical to NewLexerATNConfig6
	return NewLexerATNConfig6(state, alt, context)
}

// LHash computes hash for Lexer ATNConfigs.
func (a *ATNConfig) LHash() int {
	contextHash := 0
	if a.context != nil { contextHash = a.context.Hash() }

	semanticContextHash := 0
	if a.semanticContext != nil { semanticContextHash = a.semanticContext.Hash() }

	lexerActionExecutorHash := 0
	if a.lexerActionExecutor != nil { lexerActionExecutorHash = a.lexerActionExecutor.Hash() }

    stateNumber := 0
	if a.state != nil { stateNumber = a.state.GetStateNumber() }

	decisionVal := 0
	if a.passedThroughNonGreedyDecision { decisionVal = 1 }

	h := murmurInit(7) // Seed
	h = murmurUpdate(h, stateNumber)
	h = murmurUpdate(h, a.alt)
	h = murmurUpdate(h, contextHash)
	h = murmurUpdate(h, semanticContextHash)
	h = murmurUpdate(h, decisionVal)
	h = murmurUpdate(h, lexerActionExecutorHash)
    // Precedence filter suppressed was not in original LHash, and typically not for lexers.
    // If LEquals checks it (via PEquals), it should be here.
    // Original LEquals called PEquals. PEquals checks precedenceFilterSuppressed.
    // So, for consistency, it should be part of LHash too.
    // if a.precedenceFilterSuppressed { h = murmurUpdate(h, 1) } else { h = murmurUpdate(h, 0) }
    // return murmurFinish(h, 7) // If added, 7 items
	return murmurFinish(h, 6) // Original LHash had 6 items.
}

// lEquals performs comparison for Lexer ATNConfigs.
// Assumes 'a' and 'other' are non-nil and cType is lexerConfig for both.
func (a *ATNConfig) lEquals(other *ATNConfig) bool {
	if a.passedThroughNonGreedyDecision != other.passedThroughNonGreedyDecision {
		return false
	}

	// Compare lexerActionExecutor
	if a.lexerActionExecutor == nil {
		if other.lexerActionExecutor != nil { return false }
	} else if !a.lexerActionExecutor.Equals(other.lexerActionExecutor) { // Assumes lexerActionExecutor.Equals handles nil other
		return false
	}

	// Now compare common fields, similar to pEquals
	var contextEquals bool
	if a.context == nil {
		contextEquals = (other.context == nil)
	} else {
		contextEquals = a.context.Equals(other.context)
	}

    var stateEquals bool
	if a.state == nil {
	    stateEquals = (other.state == nil)
    } else if other.state == nil {
        stateEquals = false
    } else {
        stateEquals = (a.state.GetStateNumber() == other.state.GetStateNumber())
    }

    var semContextEquals bool
    if a.semanticContext == nil {
        semContextEquals = (other.semanticContext == nil)
    } else {
        semContextEquals = a.semanticContext.Equals(other.semanticContext)
    }

	// The original LEquals called PEquals, which includes precedenceFilterSuppressed.
	// So, we should check it here for consistency.
	return stateEquals &&
		a.alt == other.alt &&
		contextEquals &&
		semContextEquals &&
		a.precedenceFilterSuppressed == other.precedenceFilterSuppressed
}


// LEquals is the exported version.
func (a *ATNConfig) LEquals(o Collectable[*ATNConfig]) bool {
	other, ok := o.(*ATNConfig)
	if !ok || other == nil { return false }
    if a == other { return true }
    if a.cType != lexerConfig || other.cType != lexerConfig { return false } // Both must be lexer configs
    return a.lEquals(other)
}


func checkNonGreedyDecision(source *ATNConfig, target ATNState) bool {
	passedThrough := false
	if source != nil { // source can be an ATNConfig
		passedThrough = source.passedThroughNonGreedyDecision
	}

	decisionState, ok := target.(DecisionState) // target is an ATNState
	return passedThrough || (ok && decisionState.getNonGreedy())
}

[end of runtime/Go/antlr/v4/atn_config.go]
