// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"fmt"
	"sync"
)

// PredPrediction maps a predicate to a predicted alternative.
type PredPrediction struct {
	alt  int
	pred SemanticContext
}

var predPredictionPool = sync.Pool{
	New: func() interface{} {
		return new(PredPrediction)
	},
}

func (p *PredPrediction) Reset() {
	p.alt = 0 // Typically set by NewPredPrediction
	p.pred = nil // Typically set by NewPredPrediction (SemanticContextNone could be an alternative if meaningful)
}

func NewPredPrediction(pred SemanticContext, alt int) *PredPrediction {
	pp := predPredictionPool.Get().(*PredPrediction)
	pp.Reset()
	pp.alt = alt
	pp.pred = pred
	return pp
}

func (p *PredPrediction) String() string {
	return "(" + fmt.Sprint(p.pred) + ", " + fmt.Sprint(p.alt) + ")"
}

// DFAState represents a set of possible [ATN] configurations. As Aho, Sethi,
// Ullman p. 117 says: "The DFA uses its state to keep track of all possible
// states the ATN can be in after reading each input symbol. That is to say,
// after reading input a1, a2,..an, the DFA is in a state that represents the
// subset T of the states of the ATN that are reachable from the ATN's start
// state along some path labeled a1a2..an."
//
// In conventional NFA-to-DFA conversion, therefore, the subset T would be a bitset representing the set of
// states the [ATN] could be in. We need to track the alt predicted by each state
// as well, however. More importantly, we need to maintain a stack of states,
// tracking the closure operations as they jump from rule to rule, emulating
// rule invocations (method calls). I have to add a stack to simulate the proper
// lookahead sequences for the underlying LL grammar from which the ATN was
// derived.
//
// I use a set of [ATNConfig] objects, not simple states. An [ATNConfig] is both a
// state (ala normal conversion) and a [RuleContext] describing the chain of rules
// (if any) followed to arrive at that state.
//
// A [DFAState] may have multiple references to a particular state, but with
// different [ATN] contexts (with same or different alts) meaning that state was
// reached via a different set of rule invocations.
type DFAState struct {
	stateNumber int
	configs     *ATNConfigSet

	// edges elements point to the target of the symbol. Shift up by 1 so (-1)
	// Token.EOF maps to the first element.
	edges []*DFAState

	isAcceptState bool

	// prediction is the 'ttype' we match or alt we predict if the state is 'accept'.
	// Set to ATN.INVALID_ALT_NUMBER when predicates != nil or
	// requiresFullContext.
	prediction int

	lexerActionExecutor *LexerActionExecutor

	// requiresFullContext indicates it was created during an SLL prediction that
	// discovered a conflict between the configurations in the state. Future
	// ParserATNSimulator.execATN invocations immediately jump doing
	// full context prediction if true.
	requiresFullContext bool

	// predicates is the predicates associated with the ATN configurations of the
	// DFA state during SLL parsing. When we have predicates, requiresFullContext
	// is false, since full context prediction evaluates predicates on-the-fly. If
	// d is
	// not nil, then prediction is ATN.INVALID_ALT_NUMBER.
	//
	// We only use these for non-requiresFullContext but conflicting states. That
	// means we know from the context (it's $ or we don't dip into outer context)
	// that it's an ambiguity not a conflict.
	//
	// This list is computed by
	// ParserATNSimulator.predicateDFAState.
	predicates []*PredPrediction
}

var dfaStatePool = sync.Pool{
	New: func() interface{} {
		return new(DFAState)
	},
}

func (d *DFAState) Reset() {
	d.stateNumber = 0 // Or -1, but typically set by NewDFAState
	d.configs = nil     // Will be set by NewDFAState
	d.edges = nil       // Clears the slice, releases underlying array if no other refs
	d.isAcceptState = false
	d.prediction = ATNInvalidAltNumber // Use the defined invalid alt number
	d.lexerActionExecutor = nil
	d.requiresFullContext = false
	d.predicates = nil // Clears the slice
}

func NewDFAState(stateNumber int, configs *ATNConfigSet) *DFAState {
	d := dfaStatePool.Get().(*DFAState)
	d.Reset()

	d.stateNumber = stateNumber
	if configs == nil {
		// TODO: ATNConfigSet itself could be pooled if it's frequently created here.
		// For now, stick to the original logic of creating a new one if nil.
		d.configs = NewATNConfigSet(false)
	} else {
		d.configs = configs
	}
	// Other fields (edges, isAcceptState, etc.) are initialized by Reset
	// and set later during DFA construction/simulation.
	return d
}

// GetAltSet gets the set of all alts mentioned by all ATN configurations in d.
func (d *DFAState) GetAltSet() []int {
	var alts []int

	if d.configs != nil {
		for _, c := range d.configs.configs {
			alts = append(alts, c.GetAlt())
		}
	}

	if len(alts) == 0 {
		return nil
	}

	return alts
}

func (d *DFAState) getEdges() []*DFAState {
	return d.edges
}

func (d *DFAState) numEdges() int {
	return len(d.edges)
}

func (d *DFAState) getIthEdge(i int) *DFAState {
	return d.edges[i]
}

func (d *DFAState) setEdges(newEdges []*DFAState) {
	d.edges = newEdges
}

func (d *DFAState) setIthEdge(i int, edge *DFAState) {
	d.edges[i] = edge
}

func (d *DFAState) setPrediction(v int) {
	d.prediction = v
}

func (d *DFAState) String() string {
	var s string
	if d.isAcceptState {
		if d.predicates != nil {
			s = "=>" + fmt.Sprint(d.predicates)
		} else {
			s = "=>" + fmt.Sprint(d.prediction)
		}
	}

	return fmt.Sprintf("%d:%s%s", d.stateNumber, fmt.Sprint(d.configs), s)
}

func (d *DFAState) Hash() int {
	h := murmurInit(7)
	h = murmurUpdate(h, d.configs.Hash())
	return murmurFinish(h, 1)
}

// Equals returns whether d equals other. Two DFAStates are equal if their ATN
// configuration sets are the same. This method is used to see if a state
// already exists.
//
// Because the number of alternatives and number of ATN configurations are
// finite, there is a finite number of DFA states that can be processed. This is
// necessary to show that the algorithm terminates.
//
// Cannot test the DFA state numbers here because in
// ParserATNSimulator.addDFAState we need to know if any other state exists that
// has d exact set of ATN configurations. The stateNumber is irrelevant.
func (d *DFAState) Equals(o Collectable[*DFAState]) bool {
	if d == o {
		return true
	}

	return d.configs.Equals(o.(*DFAState).configs)
}
