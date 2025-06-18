// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"fmt"
	"strconv"
	"sync"
)

var (
	_emptyPredictionContextHash int
	predictionContextPool       = sync.Pool{
		New: func() interface{} {
			pc := &PredictionContext{}
			// Pre-allocate small slices if desired, e.g.:
			// pc.parents = make([]*PredictionContext, 0, 2)
			// pc.returnStates = make([]int, 0, 2)
			return pc
		},
	}
)

// BasePredictionContextEMPTY is the global empty prediction context.
// It's initialized after relevant functions and types are defined.
var BasePredictionContextEMPTY *PredictionContext

func init() {
	_emptyPredictionContextHash = murmurInit(1) // Seed for hash calculation
	_emptyPredictionContextHash = murmurFinish(_emptyPredictionContextHash, 0) // Hash of "empty"

	// Initialize the global EMPTY instance.
	// This specific instance should NOT be pooled/reset.
	BasePredictionContextEMPTY = &PredictionContext{
		cachedHash:   _emptyPredictionContextHash,
		pcType:       PredictionContextEmpty,
		returnState:  BasePredictionContextEmptyReturnState,
		parentCtx:    nil,
		parents:      nil, // Explicitly nil for the canonical empty
		returnStates: nil, // Explicitly nil for the canonical empty
	}
}

func calculateEmptyHash() int {
	return _emptyPredictionContextHash
}

const (
	BasePredictionContextEmptyReturnState = 0x7FFFFFFF
)

//goland:noinspection GoUnusedGlobalVariable
var (
	BasePredictionContextglobalNodeCount = 1 // Used by some ATN logic, not directly by PredictionContext pooling
	BasePredictionContextid              = BasePredictionContextglobalNodeCount
)

const (
	PredictionContextEmpty = iota
	PredictionContextSingleton
	PredictionContextArray
)

type PredictionContext struct {
	cachedHash   int
	pcType       int
	parentCtx    *PredictionContext // For Singleton type
	returnState  int                // For Singleton and Empty type
	parents      []*PredictionContext // For Array type
	returnStates []int                // For Array type
}

func (p *PredictionContext) Reset() {
	p.cachedHash = 0
	p.pcType = 0 // Will be overwritten by constructors
	p.parentCtx = nil
	p.returnState = 0 // Will be overwritten

	// For slices, nil out elements to help GC, then reset length to 0, retaining capacity.
	for i := range p.parents {
		p.parents[i] = nil
	}
	p.parents = p.parents[:0]

	p.returnStates = p.returnStates[:0]
}

func releasePredictionContext(p *PredictionContext) {
	if p == nil || p == BasePredictionContextEMPTY { // Critical: Do not pool the global EMPTY instance or nil.
		return
	}
	p.Reset()
	predictionContextPool.Put(p)
}

func NewEmptyPredictionContext() *PredictionContext {
	// This function is primarily for creating the BasePredictionContextEMPTY instance.
	// Regular code should use BasePredictionContextEMPTY directly.
	// If ever called to create other "empty" instances, they would be distinct.
	nep := predictionContextPool.Get().(*PredictionContext)
	nep.cachedHash = _emptyPredictionContextHash
	nep.pcType = PredictionContextEmpty
	nep.returnState = BasePredictionContextEmptyReturnState
	nep.parentCtx = nil
	nep.parents = nep.parents[:0]       // Ensure slice is empty
	nep.returnStates = nep.returnStates[:0] // Ensure slice is empty
	return nep
}

func NewBaseSingletonPredictionContext(parent *PredictionContext, returnState int) *PredictionContext {
	pc := predictionContextPool.Get().(*PredictionContext)
	pc.pcType = PredictionContextSingleton
	pc.returnState = returnState
	pc.parentCtx = parent

	if parent != nil {
		pc.cachedHash = calculateHash(parent, returnState)
	} else { // Singleton with nil parent
		pc.cachedHash = calculateEmptyHash()
	}
	// Ensure array fields are empty
	pc.parents = pc.parents[:0]
	pc.returnStates = pc.returnStates[:0]
	return pc
}

func SingletonBasePredictionContextCreate(parent *PredictionContext, returnState int) *PredictionContext {
	if returnState == BasePredictionContextEmptyReturnState && parent == nil {
		return BasePredictionContextEMPTY // Return the canonical global EMPTY instance
	}
	return NewBaseSingletonPredictionContext(parent, returnState)
}

func NewArrayPredictionContext(parents []*PredictionContext, returnStates []int) *PredictionContext {
	nec := predictionContextPool.Get().(*PredictionContext)
	nec.pcType = PredictionContextArray
	nec.parents = parents       // Assumes ownership of the provided slices
	nec.returnStates = returnStates // Assumes ownership of the provided slices

	// Ensure singleton fields are clear
	nec.parentCtx = nil
	nec.returnState = 0 // Or an invalid/default state marker if 0 is a valid returnState

	// Calculate hash
	hash := murmurInit(1)
	for _, pVal := range parents {
		if pVal != nil {
			hash = murmurUpdate(hash, pVal.Hash())
		} else {
			hash = murmurUpdate(hash, 0) // Consistent hash for nil parent in array
		}
	}
	for _, rsVal := range returnStates {
		hash = murmurUpdate(hash, rsVal)
	}
	// The number of items contributing to the hash.
	// Original Go port used `len(parents) << 1`.
	// A more standard Murmur approach is number of elements.
	// Let's use sum of lengths for clarity, assuming this matches ANTLR's intent for uniqueness.
	nec.cachedHash = murmurFinish(hash, len(parents)+len(returnStates))
	return nec
}

func (p *PredictionContext) Hash() int {
	return p.cachedHash
}

func (p *PredictionContext) Equals(other Collectable[*PredictionContext]) bool {
	if p == other {
		return true
	}
	otherP, ok := other.(*PredictionContext)
	if !ok || otherP == nil {
		return false
	}

	if p.pcType != otherP.pcType {
		return false
	}
	if p.cachedHash != otherP.cachedHash { // Hash is a strong first check
		return false
	}

	// Types and hashes are same, now type-specific comparison
	switch p.pcType {
	case PredictionContextEmpty:
		return true // All empty contexts are equal (primarily BasePredictionContextEMPTY)
	case PredictionContextSingleton:
		return p.singletonEquals(otherP)
	case PredictionContextArray:
		return p.arrayEquals(otherP)
	}
	return false
}

func (p *PredictionContext) arrayEquals(other *PredictionContext) bool {
	// Assumes: p, other non-nil, pcType is PredictionContextArray, hashes match.
	return intSlicesEqual(p.returnStates, other.returnStates) &&
		pcSliceEqual(p.parents, other.parents) // pcSliceEqual needs to handle nil elements if they occur
}

func (p *PredictionContext) singletonEquals(other *PredictionContext) bool {
	// Assumes: p, other non-nil, pcType is PredictionContextSingleton, hashes match.
	if p.returnState != other.returnState {
		return false
	}
	if p.parentCtx == nil {
		return other.parentCtx == nil
	}
	return p.parentCtx.Equals(other.parentCtx) // Recursive
}

func (p *PredictionContext) GetParent(i int) *PredictionContext {
	switch p.pcType {
	case PredictionContextSingleton:
		return p.parentCtx // Index `i` is ignored for singletons
	case PredictionContextArray:
		if i >= 0 && i < len(p.parents) {
			return p.parents[i]
		}
		return nil // Index out of bounds
	case PredictionContextEmpty:
		return nil
	}
	return nil
}

func (p *PredictionContext) getReturnState(i int) int { // Note: unexported in original, but used by PredictionContextCache
	switch p.pcType {
	case PredictionContextSingleton, PredictionContextEmpty:
		return p.returnState // Index `i` ignored
	case PredictionContextArray:
		if i >= 0 && i < len(p.returnStates) {
			return p.returnStates[i]
		}
		// Consider returning a sentinel like ATNInvalidStateNumber or panicking for out of bounds
		return 0 // Default/zero value if out of bounds
	}
	return 0 // Should be unreachable if pcType is valid
}

func (p *PredictionContext) GetReturnStates() []int {
	switch p.pcType {
	case PredictionContextArray:
		return p.returnStates // Returns internal slice directly
	case PredictionContextSingleton, PredictionContextEmpty:
		// Create a new slice for consistency, but this is an allocation.
		// API users should be aware.
		return []int{p.returnState}

	}
	return nil // Should be unreachable
}

func (p *PredictionContext) length() int {
	if p.pcType == PredictionContextArray {
		return len(p.returnStates)
	}
	return 1 // Singletons and Empty are conceptually length 1
}

func (p *PredictionContext) isEmpty() bool {
	// True if it's the canonical EMPTY instance or an equivalent structure.
	if p == BasePredictionContextEMPTY {
		return true
	}
	switch p.pcType {
	case PredictionContextEmpty:
		return true // Any context explicitly typed as Empty
	case PredictionContextSingleton:
		// A singleton is "empty" if it mirrors BasePredictionContextEMPTY structure
		return p.returnState == BasePredictionContextEmptyReturnState && p.parentCtx == nil
	case PredictionContextArray:
		// An array is "empty" if it represents the merged "$" path.
		// This means one entry: (nil parent, EmptyReturnState).
		return len(p.returnStates) == 1 && p.returnStates[0] == BasePredictionContextEmptyReturnState &&
			len(p.parents) == 1 && p.parents[0] == nil
	}
	return false
}

func (p *PredictionContext) hasEmptyPath() bool {
    if p.isEmpty() { // If the context itself is the empty representation
        return true
    }
    switch p.pcType {
    case PredictionContextSingleton:
        // A non-EMPTY singleton has an empty path if its state is the marker
        // AND its parent also has an empty path (or is nil, which is effectively BasePredictionContextEMPTY).
        // This interpretation differs from just checking p.returnState.
        // Original Java: `return returnState == EMPTY_RETURN_STATE;` for Singleton.
        // Let's match original Java for hasEmptyPath behavior for Singleton:
        return p.returnState == BasePredictionContextEmptyReturnState
    case PredictionContextArray:
        // An array has an empty path if one of its elements is EMPTY_RETURN_STATE.
        // Original Java: `return getReturnState(size() - 1) == EMPTY_RETURN_STATE;`
        // (assuming EMPTY_RETURN_STATE, if present, is sorted to the end).
        if len(p.returnStates) == 0 { return false }
        return p.returnStates[len(p.returnStates)-1] == BasePredictionContextEmptyReturnState
    }
    return false // Should not be reached if pcType is valid
}


func (p *PredictionContext) String() string {
	// Efficiently build string, e.g., using strings.Builder if available/performant for many calls
	switch p.pcType {
	case PredictionContextEmpty:
		return "$"
	case PredictionContextSingleton:
		parentStr := ""
		if p.parentCtx != nil {
			parentStr = p.parentCtx.String() // Recursive
		}
		if parentStr == "" || parentStr == "$" && p.parentCtx.isEmpty() { // Don't add " $" if parent is just EMPTY
			if p.returnState == BasePredictionContextEmptyReturnState {
				return "$" // Singleton representing only EMPTY_RETURN_STATE
			}
			return strconv.Itoa(p.returnState)
		}
		return strconv.Itoa(p.returnState) + " " + parentStr
	case PredictionContextArray:
		if len(p.returnStates) == 0 { return "[]" }
		s := "["
		for i := 0; i < len(p.returnStates); i++ {
			if i > 0 { s += ", " }
			if p.returnStates[i] == BasePredictionContextEmptyReturnState {
				s += "$"
			} else {
				s += strconv.Itoa(p.returnStates[i])
			}
			// Parent part of the string
			if i < len(p.parents) && p.parents[i] != nil {
				parentStr := p.parents[i].String()
                // Avoid " $" if parent is truly empty and not just a nested structure ending in $
				if !(parentStr == "$" && p.parents[i].isEmpty()) {
					s += " " + parentStr
				} else if parentStr == "$" && p.parents[i].isEmpty() && p.parents[i] != BasePredictionContextEMPTY {
                    // If it's a complex structure that results in "$", but isn't THE BasePredictionContextEMPTY
                    s += " " + parentStr
                }
			} else if i < len(p.parents) && p.parents[i] == nil {
				// If there's an explicit nil parent in the array (e.g. for merged $ path)
				// The original Java code implies this structure: `(state parent)`
				// and if parent is EMPTY, it shows `(state $)`. It doesn't show `(state nil)`.
				// So, if parent is nil (representing EMPTY for this path), it's often omitted or shown as $.
				// Let's omit if parent is nil, matching typical representation of array path $ (nil parent)
			}
		}
		s += "]"
		return s
	}
	return "unknown"
}


func (p *PredictionContext) Type() int {
	return p.pcType
}

func calculateHash(parent *PredictionContext, returnState int) int {
	h := murmurInit(1)
	parentHash := 0
	if parent != nil {
		parentHash = parent.Hash()
	}
	h = murmurUpdate(h, parentHash)
	h = murmurUpdate(h, returnState)
	return murmurFinish(h, 2) // Hashed 2 items: parentHash, returnState
}

func predictionContextFromRuleContext(a *ATN, outerContext RuleContext) *PredictionContext {
	if outerContext == nil || outerContext.GetParent() == nil || outerContext == ParserRuleContextEmpty {
		return BasePredictionContextEMPTY
	}
	parentPC := predictionContextFromRuleContext(a, outerContext.GetParent().(RuleContext))

	invokingStateNum := outerContext.GetInvokingState()
    if invokingStateNum < 0 || invokingStateNum >= len(a.states) { return BasePredictionContextEMPTY }
	state := a.states[invokingStateNum]
	if state == nil || len(state.GetTransitions()) == 0 { return BasePredictionContextEMPTY }

	transition := state.GetTransitions()[0] // Assuming the first transition is the rule call
	ruleTransition, ok := transition.(*RuleTransition)
	if !ok { return BasePredictionContextEMPTY } // Should be a RuleTransition

	return SingletonBasePredictionContextCreate(parentPC, ruleTransition.followState.GetStateNumber())
}

func merge(a, b *PredictionContext, rootIsWildcard bool, mergeCache *JPCMap) *PredictionContext {
	if a == b || (a != nil && a.Equals(b)) { // Pointer equality or logical equality
		return a
	}

	if mergeCache != nil {
		if cached, present := mergeCache.Get(a, b); present { return cached }
		if cached, present := mergeCache.Get(b, a); present { return cached }
	}

	var result *PredictionContext
	// Handle EMPTY merging explicitly first, as it has specific rules
    isAEmpty := a.isEmpty() // Use isEmpty for logical check
    isBEmpty := b.isEmpty()

    if isAEmpty && isBEmpty { result = BasePredictionContextEMPTY } else
    if isAEmpty { result = mergeRoot(a,b,rootIsWildcard) } else // Let mergeRoot handle $ + x
    if isBEmpty { result = mergeRoot(a,b,rootIsWildcard) } else // Let mergeRoot handle x + $
    // Standard merge if neither is inherently empty (though they might contain empty paths)
	if result == nil { // If mergeRoot didn't resolve
		if a.pcType == PredictionContextSingleton && b.pcType == PredictionContextSingleton {
			result = mergeSingletons(a, b, rootIsWildcard, mergeCache)
		} else { // At least one is an array (or will be converted)
			var ara, arb *PredictionContext
			var araIsTmp, arbIsTmp bool

			if a.pcType == PredictionContextArray { ara = a } else { ara = convertToArray(a); araIsTmp = true }
			if b.pcType == PredictionContextArray { arb = b } else { arb = convertToArray(b); arbIsTmp = true }

			result = mergeArrays(ara, arb, rootIsWildcard, mergeCache)

			if araIsTmp && ara != result { releasePredictionContext(ara) }
			if arbIsTmp && arb != result { releasePredictionContext(arb) }
		}
	}

	if mergeCache != nil && result != nil { mergeCache.Put(a, b, result) }
	return result
}

func convertToArray(pc *PredictionContext) *PredictionContext {
	switch pc.Type() {
	case PredictionContextEmpty:
		return NewArrayPredictionContext(
			[]*PredictionContext{nil}, // Represents the parent of EMPTY_RETURN_STATE in an array
			[]int{BasePredictionContextEmptyReturnState},
		)
	case PredictionContextSingleton:
		return NewArrayPredictionContext(
			[]*PredictionContext{pc.parentCtx},
			[]int{pc.returnState},
		)
	default: // Already Array
		return pc
	}
}

func mergeSingletons(a, b *PredictionContext, rootIsWildcard bool, mergeCache *JPCMap) *PredictionContext {
    // mergeRoot has already been tried if one of them was EMPTY.
    // So a and b are non-EMPTY singletons here.
	if a.returnState == b.returnState {
		mergedParent := merge(a.parentCtx, b.parentCtx, rootIsWildcard, mergeCache)
		if mergedParent == a.parentCtx { return a }
		if mergedParent == b.parentCtx { return b }
		return SingletonBasePredictionContextCreate(mergedParent, a.returnState) // New pooled singleton
	}

	// Different return states
	var commonParent *PredictionContext
    if a.parentCtx == b.parentCtx || (a.parentCtx != nil && a.parentCtx.Equals(b.parentCtx)) {
        commonParent = a.parentCtx
    }

	payloads := make([]int, 2)
	parents := make([]*PredictionContext, 2)

	if commonParent != nil { // Different states, common parent -> array with common parent
		parents[0], parents[1] = commonParent, commonParent
	} else { // Different states, different parents -> array with distinct parents
		parents[0], parents[1] = a.parentCtx, b.parentCtx // Will be sorted along with payloads
	}
    // Sort by returnState to ensure canonical array form
	if a.returnState < b.returnState {
		payloads[0], payloads[1] = a.returnState, b.returnState
        if commonParent == nil { // Only swap parents if they were not common
            parents[0], parents[1] = a.parentCtx, b.parentCtx
        }
	} else {
		payloads[0], payloads[1] = b.returnState, a.returnState
        if commonParent == nil {
            parents[0], parents[1] = b.parentCtx, a.parentCtx
        }
	}
	return NewArrayPredictionContext(parents, payloads) // New pooled array
}

func mergeRoot(a, b *PredictionContext, rootIsWildcard bool) *PredictionContext {
    // This function is for when one of a or b is EMPTY.
    // isAEmpty/isBEmpty should use the isEmpty() method for logical emptiness.
    isAEmpty := a.isEmpty()
    isBEmpty := b.isEmpty()

	if rootIsWildcard {
		if isAEmpty || isBEmpty { return BasePredictionContextEMPTY } // $ + x = $, x + $ = $ (wildcard)
	} else { // Full context merge
		if isAEmpty && isBEmpty { return BasePredictionContextEMPTY } // $ + $ = $

        // If one is empty, create an array [non-empty, $ representation]
        // The $ representation in an array is (nil parent, EMPTY_RETURN_STATE)
        var nonEemptyCtx *PredictionContext
        if isAEmpty { nonEemptyCtx = b } else { nonEemptyCtx = a }

        // Create slices for the new array context.
        // Order convention: often $ comes first, or sorted.
        // Let's go with [non-empty-path, empty-path-marker] for now, then sort if needed.
        // ANTLR Java sorts these, typically $ (EMPTY_RETURN_STATE) comes after actual states.

        payloads := make([]int, 2)
        parents := make([]*PredictionContext, 2)

        // Path 1: from the non-empty context (which must be a singleton here if mergeRoot is called this way)
        payloads[0] = nonEemptyCtx.returnState
        parents[0] = nonEemptyCtx.parentCtx

        // Path 2: the empty path marker
        payloads[1] = BasePredictionContextEmptyReturnState
        parents[1] = nil // Parent of EMPTY_RETURN_STATE marker in array is nil

        // Sort them: EMPTY_RETURN_STATE is large, so it usually comes last if sorted numerically.
        if payloads[0] > payloads[1] {
            payloads[0], payloads[1] = payloads[1], payloads[0]
            parents[0], parents[1] = parents[1], parents[0]
        }
        return NewArrayPredictionContext(parents, payloads) // New pooled array
	}
	return nil // No resolution by mergeRoot (e.g. neither was empty)
}


func mergeArrays(a, b *PredictionContext, rootIsWildcard bool, mergeCache *JPCMap) *PredictionContext {
    // Ensure a and b are array types for this specialized merge.
    // Temporary slices for building the result, initial capacity is sum of lengths.
    mergedRS := make([]int, 0, len(a.returnStates)+len(b.returnStates))
    mergedP := make([]*PredictionContext, 0, len(a.parents)+len(b.parents))

    idxA, idxB := 0, 0
    for idxA < len(a.returnStates) && idxB < len(b.returnStates) {
        sA, pA := a.returnStates[idxA], a.parents[idxA]
        sB, pB := b.returnStates[idxB], b.parents[idxB]

        if sA == sB {
            mp := merge(pA, pB, rootIsWildcard, mergeCache) // Merge parents
            mergedRS = append(mergedRS, sA)
            mergedP = append(mergedP, mp)
            idxA++
            idxB++
        } else if sA < sB {
            mergedRS = append(mergedRS, sA)
            mergedP = append(mergedP, pA)
            idxA++
        } else { // sB < sA
            mergedRS = append(mergedRS, sB)
            mergedP = append(mergedP, pB)
            idxB++
        }
    }
    // Append remaining from a
    for idxA < len(a.returnStates) {
        mergedRS = append(mergedRS, a.returnStates[idxA])
        mergedP = append(mergedP, a.parents[idxA])
        idxA++
    }
    // Append remaining from b
    for idxB < len(b.returnStates) {
        mergedRS = append(mergedRS, b.returnStates[idxB])
        mergedP = append(mergedP, b.parents[idxB])
        idxB++
    }

    if len(mergedRS) == 0 { return BasePredictionContextEMPTY } // Should not happen if inputs are valid non-empty arrays
    if len(mergedRS) == 1 { // Result is a singleton
        // Release temporary slices if they were large? No, they are local.
        return SingletonBasePredictionContextCreate(mergedP[0], mergedRS[0]) // Pooled singleton
    }

    M := NewArrayPredictionContext(mergedP, mergedRS) // Pooled array, takes ownership of slices

    if M.Equals(a) { releasePredictionContext(M); return a }
    if M.Equals(b) { releasePredictionContext(M); return b }

    combineCommonParents(&M.parents) // Modifies M.parents in-place
    return M
}

func combineCommonParents(parents *[]*PredictionContext) {
    if len(*parents) <= 1 { return }
    // O(N^2) but often few parents or already canonical.
    // A more complex Set-like structure could optimize, but this is direct.
    uniqueList := make([]*PredictionContext, 0, len(*parents))
    for i := 0; i < len(*parents); i++ {
        currentP := (*parents)[i]
        if currentP == nil { continue } // Should not have nil parents post-merge normally

        isUnique := true
        for _, uniqueP := range uniqueList {
            if currentP.Equals(uniqueP) {
                (*parents)[i] = uniqueP // Replace with canonical instance
                isUnique = false
                break
            }
        }
        if isUnique {
            uniqueList = append(uniqueList, currentP)
        }
    }
}

func getCachedBasePredictionContext(context *PredictionContext, contextCache *PredictionContextCache, visited *VisitRecord) *PredictionContext {
	if context == nil || context.isEmpty() { // Check nil and logical empty
		return BasePredictionContextEMPTY
	}
	if existing, present := visited.Get(context); present { return existing }
	if existing, present := contextCache.Get(context); present {
		visited.Put(context, existing)
		return existing
	}

	changed := false
	currentParentsList := context.GetParentsForCaching() // Get appropriate parent(s) for this context type

	var canonicalParents []*PredictionContext // This will hold the canonical versions of parents

	if context.pcType == PredictionContextArray {
	    if len(currentParentsList) > 0 { // Only allocate if there are parents
		canonicalParents = make([]*PredictionContext, len(currentParentsList))
        } else {
            canonicalParents = nil // Or an empty slice, depending on convention for array with no parents
        }
	} else if context.pcType == PredictionContextSingleton && len(currentParentsList) == 1 { // Singleton
	    canonicalParents = make([]*PredictionContext, 1) // Expect one parent
	}


	for i, p := range currentParentsList {
		if p == nil { // Should not happen if GetParentsForCaching is correct
		    if context.pcType == PredictionContextArray { canonicalParents[i] = nil }
			continue
		}
		canonicalP := getCachedBasePredictionContext(p, contextCache, visited)
		if canonicalP != p { changed = true }

		if context.pcType == PredictionContextArray {
		    canonicalParents[i] = canonicalP
        } else if context.pcType == PredictionContextSingleton { // Singleton
            canonicalParents[0] = canonicalP // Store the single canonical parent
        }
	}

	if !changed { // No change in parents, and context itself wasn't in cache
		contextCache.add(context)
		visited.Put(context, context)
		return context
	}

	// Parents changed, or context itself needs canonicalization. Create new canonical version.
	var updated *PredictionContext
	if context.pcType == PredictionContextSingleton {
		updated = SingletonBasePredictionContextCreate(canonicalParents[0], context.returnState) // Pooled
	} else { // Array
        // NewArrayPredictionContext needs its own copy of returnStates if context.returnStates is shared
        // Create a copy of returnStates to pass to NewArrayPredictionContext
        returnStatesCopy := make([]int, len(context.returnStates))
        copy(returnStatesCopy, context.returnStates)
		updated = NewArrayPredictionContext(canonicalParents, returnStatesCopy) // Pooled
	}

	contextCache.add(updated)
	visited.Put(updated, updated) // New canonical form points to itself
	visited.Put(context, updated) // Original form now resolves to this new canonical one

	// The original `context` (if `changed`) is now non-canonical.
	// It's not released here because its original parent objects might still be part of other structures.
	// The main benefit is that `updated` is from the pool and is canonical.
	return updated
}

// GetParentsForCaching is a helper for getCachedBasePredictionContext.
// It returns a slice of parents suitable for recursive caching.
// For Singleton, it returns a slice with one element. For Array, its internal parents slice. Empty otherwise.
func (p *PredictionContext) GetParentsForCaching() []*PredictionContext {
    switch p.pcType {
    case PredictionContextSingleton:
        // For consistent processing, wrap the single parent in a slice.
        // Avoid allocation if parent is nil.
        if p.parentCtx == nil { return nil } // Or empty slice: make([]*PredictionContext,0)
        // This allocation is minor compared to overall context creation.
        return []*PredictionContext{p.parentCtx}
    case PredictionContextArray:
        return p.parents // Return internal slice directly
    case PredictionContextEmpty:
        return nil // Empty context has no parents for caching
    }
    return nil // Should be unreachable
}

// pcSliceEqual compares two slices of PredictionContext pointers.
// It must handle nil PredictionContext pointers within the slices if they can occur.
func pcSliceEqual(a, b []*PredictionContext) bool {
	if len(a) != len(b) {
		return false
	}
	for i, pcA := range a {
		pcB := b[i]
		if pcA == pcB { // Pointer equality (covers both nil)
			continue
		}
		if pcA == nil || pcB == nil { // One is nil, other is not (since not both nil via ==)
			return false
		}
		if !pcA.Equals(pcB) { // Deep equality check
			return false
		}
	}
	return true
}

// intSlicesEqual compares two slices of integers.
func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i, vA := range a {
		if vA != b[i] {
			return false
		}
	}
	return true
}
