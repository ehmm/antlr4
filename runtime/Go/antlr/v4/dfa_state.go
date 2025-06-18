// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"fmt"
	"sync"
)

var (
	predPredictionPool = sync.Pool{
		New: func() interface{} {
			return new(PredPrediction)
		},
	}
	dfaStatePool = sync.Pool{
		New: func() interface{} {
			// ds := new(DFAState)
			// ds.edges = make([]*DFAState, 0, 4) // Example initial capacity
			// ds.predicates = make([]*PredPrediction, 0, 2) // Example initial capacity
			// return ds
			return new(DFAState)
		},
	}
)

// PredPrediction maps a predicate to a predicted alternative.
type PredPrediction struct {
	alt  int
	pred SemanticContext // Usually a singleton like SemanticContextNone or a shared instance
}

func (pp *PredPrediction) Reset() {
	pp.alt = 0
	// SemanticContexts are often shared singletons (e.g., SemanticContextNone)
	// or part of a cache. Do not release them here, just nil the reference.
	pp.pred = nil
}

func releasePredPrediction(pp *PredPrediction) {
	if pp == nil {
		return
	}
	pp.Reset()
	predPredictionPool.Put(pp)
}

func NewPredPrediction(pred SemanticContext, alt int) *PredPrediction {
	pp := predPredictionPool.Get().(*PredPrediction)
	pp.alt = alt
	pp.pred = pred
	return pp
}

func (p *PredPrediction) String() string {
	// Ensure p.pred is not nil before calling Sprint on it, if that's possible.
	predStr := "nil"
	if p.pred != nil {
		predStr = fmt.Sprint(p.pred)
	}
	return "(" + predStr + ", " + fmt.Sprint(p.alt) + ")"
}

// DFAState represents a set of possible [ATN] configurations.
type DFAState struct {
	stateNumber int
	configs     *ATNConfigSet // Often shared and read-only, manage lifecycle carefully

	edges []*DFAState // References to other DFAStates, forms graph structure

	isAcceptState bool
	prediction    int

	lexerActionExecutor *LexerActionExecutor // Usually shared or part of ATN structure

	requiresFullContext bool
	predicates          []*PredPrediction // List of owned PredPrediction objects
}

func (ds *DFAState) Reset() {
	ds.stateNumber = 0 // Or a sentinel like -1 if 0 is a valid state number

	// DFAState.configs usually points to a shared, read-only ATNConfigSet.
	// It should NOT be released here. Nilling the reference is sufficient.
	// The actual ATNConfigSet lifecycle is managed elsewhere (e.g., DFA or PredictionContextCache).
	ds.configs = nil

	// Edges form the DFA graph. Resetting a state means it no longer points to others.
	// The other states are not "owned" by this state for release purposes.
	if ds.edges != nil {
		for i := range ds.edges {
			ds.edges[i] = nil // Nil out references
		}
		ds.edges = ds.edges[:0] // Reset slice, keep capacity
	}

	ds.isAcceptState = false
	ds.prediction = ATNInvalidAltNumber // Default to invalid/unset
	ds.lexerActionExecutor = nil // Reference, not owned for release here typically
	ds.requiresFullContext = false

	// Predicates are "owned" by this DFAState. Release them.
	if ds.predicates != nil {
		for _, pp := range ds.predicates {
			releasePredPrediction(pp) // Assumes releasePredPrediction handles nil
		}
		ds.predicates = ds.predicates[:0] // Reset slice, keep capacity
	}
}

func releaseDFAState(ds *DFAState) {
	if ds == nil {
		return
	}
	ds.Reset()
	dfaStatePool.Put(ds)
}

func NewDFAState(stateNumber int, configs *ATNConfigSet) *DFAState {
	ds := dfaStatePool.Get().(*DFAState)

	ds.stateNumber = stateNumber
	ds.configs = configs // This DFAState now references this ATNConfigSet

	// Ensure other fields are at their default/reset state
	// Reset should have handled slices, but ensure for clarity or if initial capacity is desired.
	if ds.edges == nil {
	    ds.edges = make([]*DFAState, 0) // Or specific initial capacity
    } else {
        ds.edges = ds.edges[:0]
    }

    if ds.predicates == nil {
        ds.predicates = make([]*PredPrediction, 0)
    } else {
        ds.predicates = ds.predicates[:0]
    }

	ds.isAcceptState = false
	ds.prediction = ATNInvalidAltNumber
	ds.lexerActionExecutor = nil
	ds.requiresFullContext = false

	// If configs was nil and this constructor was to create a new default one:
	// This behavior is slightly changed. Original NewDFAState would do:
	//   if configs == nil { configs = NewATNConfigSet(false) }
	//   return &DFAState{configs: configs, stateNumber: stateNumber}
	// Now, if configs is passed as nil, ds.configs will be nil.
	// Callers of NewDFAState must provide an ATNConfigSet if one is needed.
	// If the intent was for NewDFAState to create one if nil, that logic needs to be here:
	// if ds.configs == nil {
	//    ds.configs = NewATNConfigSet(false) // NewATNConfigSet uses its pool
	// }
	// For now, assuming caller manages configs. If configs is nil, this state has nil configs.
	return ds
}

// GetAltSet gets the set of all alts mentioned by all ATN configurations in d.
func (d *DFAState) GetAltSet() []int {
	// This method is problematic if d.configs can be nil. Add checks.
	if d.configs == nil || d.configs.configs == nil { // Check configs and its internal slice
		return nil
	}

	var alts []int // Initialized as nil, will become non-nil on first append
	for _, c := range d.configs.configs {
		if c != nil { // Guard against nil ATNConfig in the slice
			alts = append(alts, c.GetAlt())
		}
	}
	// No change to original return: if alts remains empty (e.g. no configs or all alts were such that they didn't add),
	// it will return nil if it was never appended to, or an empty slice if append was called.
	// For consistency, let's ensure it returns nil if truly no alts, or empty slice if alts were processed but none found.
	// The original logic would return `nil` if `alts` remained empty.
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
    if i < 0 || i >= len(d.edges) { return nil } // Bounds check
	return d.edges[i]
}

func (d *DFAState) setEdges(newEdges []*DFAState) {
	// This DFAState takes ownership or reference to newEdges.
	// Old edges are effectively discarded. If they contained pooled DFAStates that this state "owned",
	// they should have been released. But edges are graph links, not owned children.
	d.edges = newEdges
}

func (d *DFAState) setIthEdge(i int, edge *DFAState) {
    if i < 0 || i >= len(d.edges) {
        // Handle out of bounds: panic, extend slice, or ignore.
        // Original code doesn't show bounds checking here, implies `i` is valid.
        // For safety, one might add:
        // if i >= len(d.edges) { panic("setIthEdge: index out of bounds") }
        return // Or panic
    }
	d.edges[i] = edge
}

func (d *DFAState) setPrediction(v int) {
	d.prediction = v
}

func (d *DFAState) String() string {
	acceptStr := ""
	if d.isAcceptState {
		if d.predicates != nil && len(d.predicates) > 0 {
			// Build string for predicates carefully
			predStr := "["
			for i, p := range d.predicates {
				if p != nil { predStr += p.String() } else { predStr += "nil" }
				if i < len(d.predicates)-1 { predStr += ", " }
			}
			predStr += "]"
			acceptStr = "=>" + predStr
		} else {
			acceptStr = "=>" + fmt.Sprint(d.prediction)
		}
	}
    configsStr := "nil"
    if d.configs != nil {
        configsStr = fmt.Sprint(d.configs)
    }
	return fmt.Sprintf("%d:%s%s", d.stateNumber, configsStr, acceptStr)
}

func (d *DFAState) Hash() int {
	// Hash must be based on immutable fields or fields that define equality.
	// DFAState equality is based on its ATNConfigSet.
	if d.configs == nil { return 0 } // Or a specific hash for nil configs
	h := murmurInit(7)
	h = murmurUpdate(h, d.configs.Hash())
	return murmurFinish(h, 1) // Hashed 1 item (the config set's hash)
}

// Equals returns whether d equals other. Two DFAStates are equal if their ATN
// configuration sets are the same. stateNumber is not part of equality.
func (d *DFAState) Equals(o Collectable[*DFAState]) bool {
	if d == o { return true }
	other, ok := o.(*DFAState)
	if !ok || other == nil { return false }

	// Equality is based on the ATNConfigSet `configs`.
	if d.configs == nil {
		return other.configs == nil
	}
	return d.configs.Equals(other.configs) // Relies on ATNConfigSet.Equals
}

[end of runtime/Go/antlr/v4/dfa_state.go]
