// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"fmt"
	"sync"
)

var (
	atnConfigSetPool = sync.Pool{
		New: func() interface{} {
			// Initialize with a default capacity for configs slice if desired
			// cs := new(ATNConfigSet)
			// cs.configs = make([]*ATNConfig, 0, 10) // Example capacity
			// return cs
			return new(ATNConfigSet)
		},
	}
)

// ATNConfigSet is a specialized set of ATNConfig that tracks information
// about its elements and can combine similar configurations using a
// graph-structured stack.
type ATNConfigSet struct {
	cachedHash int

	// configLookup is used to determine whether two ATNConfigSets are equal. We
	// need all configurations with the same (s, i, _, semctx) to be equal. A key
	// effectively doubles the number of objects associated with ATNConfigs. All
	// keys are hashed by (s, i, _, pi), not including the context. Wiped out when
	// read-only because a set becomes a DFA state.
	configLookup *JStore[*ATNConfig, Comparator[*ATNConfig]]

	// configs is the added elements that did not match an existing key in configLookup
	configs []*ATNConfig

	conflictingAlts *BitSet // Can be nil

	dipsIntoOuterContext bool
	fullCtx              bool
	hasSemanticContext   bool
	readOnly             bool
	uniqueAlt            int
}

func (cs *ATNConfigSet) Reset() {
	cs.cachedHash = -1 // Reset cached hash

	// Release all ATNConfig objects held by this set if they are pooled.
	// This is crucial for cascading pooling.
	if cs.configs != nil {
		for _, cfg := range cs.configs {
			releaseATNConfig(cfg) // Assumes releaseATNConfig handles nil
		}
		cs.configs = cs.configs[:0] // Reset slice length, keep capacity
	}

	// configLookup also holds ATNConfig references.
	// The JStore.Clear() would remove them from the store.
	// If JStore doesn't own them (they are same as in cs.configs), releasing above is enough.
	// If JStore could hold different ATNConfig instances, they'd need separate release.
	// Typically, configLookup stores the same ATNConfig instances that are in the configs slice.
	// So, releasing them via cs.configs loop should be sufficient.
	// We still need to clear or replace the JStore itself.
	if cs.configLookup != nil {
		cs.configLookup.Clear() // Clear the JStore; it might have its own pooling for internal nodes if complex.
		// For now, assume JStore itself is not pooled, but its contents (ATNConfigs) are.
	}
	// cs.configLookup = nil // Or re-initialize if New... methods expect a non-nil one.

	if cs.conflictingAlts != nil {
		cs.conflictingAlts.clear() // Assuming BitSet has a Clear method or can be reset.
		// If BitSet is pooled, releaseConflictingAlts(cs.conflictingAlts)
		// For now, assume BitSet is not pooled, just cleared.
	}
	cs.conflictingAlts = nil // Or re-initialize if needed

	cs.dipsIntoOuterContext = false
	cs.fullCtx = false // Default for new sets, will be set by constructor
	cs.hasSemanticContext = false
	cs.readOnly = false
	cs.uniqueAlt = ATNInvalidAltNumber // Default invalid alt
}

func releaseATNConfigSet(cs *ATNConfigSet) {
	if cs == nil {
		return
	}
	cs.Reset()
	atnConfigSetPool.Put(cs)
}

// Alts returns the combined set of alts for all the configurations in this set.
func (cs *ATNConfigSet) Alts() *BitSet {
	alts := NewBitSet() // NewBitSet might be a candidate for pooling if frequently used
	for _, it := range cs.configs {
		alts.add(it.GetAlt())
	}
	return alts
}

// NewATNConfigSet creates a new ATNConfigSet instance.
func NewATNConfigSet(fullCtx bool) *ATNConfigSet {
	cs := atnConfigSetPool.Get().(*ATNConfigSet)
	// Reset should have cleared most fields. Initialize specific ones.
	cs.fullCtx = fullCtx
	cs.cachedHash = -1 // Ensure hash is invalid initially
	cs.uniqueAlt = ATNInvalidAltNumber
	cs.readOnly = false
	cs.dipsIntoOuterContext = false
	cs.hasSemanticContext = false

	// Ensure slices and maps are clean (Reset should handle configs)
	if cs.configs == nil {
	    cs.configs = make([]*ATNConfig, 0) // Default initial capacity
    } else {
        cs.configs = cs.configs[:0]
    }

	// configLookup needs to be (re)initialized.
	// If JStore has internal state that needs reset or if it should be from a pool, handle here.
	// For now, assume NewJStore is fine to call.
	cs.configLookup = NewJStore[*ATNConfig, Comparator[*ATNConfig]](aConfCompInst, ATNConfigLookupCollection, "NewATNConfigSet()")

	// conflictingAlts is often nil initially.
	cs.conflictingAlts = nil // Explicitly nil, can be created on demand

	return cs
}

// Add merges contexts with existing configs for (s, i, pi, _),
// where 's' is the ATNConfig.state, 'i' is the ATNConfig.alt, and
// 'pi' is the [ATNConfig].semanticContext.
//
// We use (s,i,pi) as the key.
// Updates dipsIntoOuterContext and hasSemanticContext when necessary.
func (cs *ATNConfigSet) Add(config *ATNConfig, mergeCache *JPCMap) bool {
	if cs.readOnly {
		panic("set is read-only")
	}

	if config.GetSemanticContext() != SemanticContextNone {
		cs.hasSemanticContext = true
	}

	if config.GetReachesIntoOuterContext() > 0 {
		cs.dipsIntoOuterContext = true
	}

	existing, present := cs.configLookup.Put(config) // config is added to JStore here

	if !present { // The config was not already in the set (based on key (s,i,pi))
		cs.cachedHash = -1
		cs.configs = append(cs.configs, config) // Add to the list of configs
		// `config` is now owned by this ATNConfigSet.
		return true
	}

	// Config was already present (same key). Merge contexts.
	// `existing` is the one from configLookup. `config` is the new one being added.
	// We update `existing` and discard `config`.
	rootIsWildcard := !cs.fullCtx
	mergedContext := merge(existing.GetContext(), config.GetContext(), rootIsWildcard, mergeCache)

	existing.SetReachesIntoOuterContext(intMax(existing.GetReachesIntoOuterContext(), config.GetReachesIntoOuterContext()))

	if config.getPrecedenceFilterSuppressed() {
		existing.setPrecedenceFilterSuppressed(true)
	}

	existing.SetContext(mergedContext) // Update existing config's context

	// The incoming 'config' was not added to cs.configs because 'existing' (with same key)
	// was already there. The 'config' is now effectively temporary/discarded.
	// If 'config' was obtained from a pool, it should be released.
	releaseATNConfig(config) // Release the passed-in config as it's not stored directly.

	return true // True because the set was modified (existing config's context changed)
}

// GetStates returns the set of states represented by all configurations in this config set
func (cs *ATNConfigSet) GetStates() *JStore[ATNState, Comparator[ATNState]] {
	// JStore for states might be pooled if created very often. For now, new one each time.
	states := NewJStore[ATNState, Comparator[ATNState]](aStateEqInst, ATNStateCollection, "ATNConfigSet.GetStates()")
	for _, cfg := range cs.configs { // Iterate over current configs
		if cfg.GetState() != nil { // Guard against nil state if possible
		    states.Put(cfg.GetState())
        }
	}
	return states
}

func (cs *ATNConfigSet) GetPredicates() []SemanticContext {
	predicates := make([]SemanticContext, 0) // Fresh slice
	for _, cfg := range cs.configs {
		semCtx := cfg.GetSemanticContext()
		if semCtx != SemanticContextNone && semCtx != nil {
			predicates = append(predicates, semCtx)
		}
	}
	return predicates
}

func (cs *ATNConfigSet) OptimizeConfigs(interpreter *BaseATNSimulator) {
	if cs.readOnly {
		panic("set is read-only")
	}
	if cs.configLookup == nil || cs.configLookup.Len() == 0 {
		return
	}
	for _, config := range cs.configs {
		if config.GetContext() != nil { // Ensure context exists before getting cached version
		    config.SetContext(interpreter.getCachedContext(config.GetContext()))
        }
	}
}

// AddAll adds all configs from 'coll' to this set.
// The ATNConfig objects in 'coll' are potentially added or merged.
// If an ATNConfig from 'coll' is merged (and thus not stored directly), it's released by Add().
func (cs *ATNConfigSet) AddAll(coll []*ATNConfig) bool {
	changed := false // Track if Add operation modified the set
	for _, cfg := range coll {
		if cs.Add(cfg, nil) { // Add will release 'cfg' if it's not kept
			changed = true
		}
	}
	return changed // Return if any add operation caused a change
}

// Compare checks if this set is equal to another ATNConfigSet 'bs' based on ordered comparison of configs.
func (cs *ATNConfigSet) Compare(bs *ATNConfigSet) bool {
	if cs == bs { return true }
	if bs == nil { return false }
	if len(cs.configs) != len(bs.configs) {
		return false
	}
	for i, c1 := range cs.configs {
		c2 := bs.configs[i]
		if c1 == c2 { continue }
		if c1 == nil || c2 == nil { return false } // One nil, other not
		if !c1.Equals(c2) {
			return false
		}
	}
	return true
}

// Equals checks for logical equality with another ATNConfigSet.
// This considers more than just the ordered list of configs (e.g., fullCtx, uniqueAlt).
func (cs *ATNConfigSet) Equals(other Collectable[ATNConfig]) bool { // Parameter was Collectable[*ATNConfig] - changed for ATNConfigSet
    if cs == other { return true }
	otherSet, ok := other.(*ATNConfigSet) // other must be ATNConfigSet
	if !ok || otherSet == nil { return false }

	// Compare essential properties first
	if cs.fullCtx != otherSet.fullCtx ||
		cs.uniqueAlt != otherSet.uniqueAlt ||
		cs.hasSemanticContext != otherSet.hasSemanticContext ||
		cs.dipsIntoOuterContext != otherSet.dipsIntoOuterContext {
		return false
	}

	// Compare conflictingAlts BitSets
	var conflictingAltsEqual bool
	if cs.conflictingAlts == nil {
		conflictingAltsEqual = (otherSet.conflictingAlts == nil)
	} else {
		conflictingAltsEqual = cs.conflictingAlts.equals(otherSet.conflictingAlts) // Assumes BitSet has equals
	}
	if !conflictingAltsEqual { return false }

	// Finally, compare the ordered list of configs
	return cs.Compare(otherSet)
}


func (cs *ATNConfigSet) Hash() int {
	if cs.readOnly && cs.cachedHash != -1 {
		return cs.cachedHash
	}
	// Calculate hash based on the ordered list of configs.
	// This matches the Compare method's primary focus.
	// Other fields (fullCtx, uniqueAlt etc.) are part of Equals but not typically part of this specific hash.
	// If those fields should contribute to hashing for non-readonly sets, this needs adjustment.
	// Original just hashed configs.
	h := 1
	for _, config := range cs.configs {
		if config != nil { // Guard against nil config in slice if possible
		    h = 31*h + config.Hash()
        }
	}
	// If readOnly, cache it.
	if cs.readOnly {
		cs.cachedHash = h
	}
	return h
}

// hashCodeConfigs was the original internal hashing method.
// func (cs *ATNConfigSet) hashCodeConfigs() int { ... }

// Contains checks if 'item' is in the configLookup.
func (cs *ATNConfigSet) Contains(item *ATNConfig) bool {
	if cs.readOnly { panic("not implemented for read-only sets") } // Or return false/error
	if cs.configLookup == nil { return false }
	return cs.configLookup.Contains(item)
}

// ContainsFast was an alias.
func (cs *ATNConfigSet) ContainsFast(item *ATNConfig) bool {
	return cs.Contains(item)
}

// Clear removes all ATNConfigs from the set.
// Pooled ATNConfigs within are released.
func (cs *ATNConfigSet) Clear() {
	if cs.readOnly {
		panic("set is read-only")
	}
	// Release contained ATNConfigs
	if cs.configs != nil {
		for _, cfg := range cs.configs {
			releaseATNConfig(cfg)
		}
		cs.configs = cs.configs[:0]
	}
	if cs.configLookup != nil {
		// JStore.Clear() removes elements. We've already released ATNConfigs via cs.configs.
		// If JStore could hold *other* ATNConfigs not in cs.configs, iterate and release JStore values too.
		// Assuming JStore values are same objects as in cs.configs.
		cs.configLookup.Clear()
	}
	cs.cachedHash = -1
	cs.hasSemanticContext = false
	cs.dipsIntoOuterContext = false
	// Other fields like uniqueAlt might need reset depending on semantics of Clear.
	// Resetting to a state similar to a freshly New'd ATNConfigSet.
}

func (cs *ATNConfigSet) String() string {
	s := "["
	for i, c := range cs.configs {
		if c != nil { s += c.String() } else { s += "nil" }
		if i != len(cs.configs)-1 { s += ", " }
	}
	s += "]"

	if cs.hasSemanticContext { s += ",hasSemanticContext=" + fmt.Sprint(cs.hasSemanticContext) }
	if cs.uniqueAlt != ATNInvalidAltNumber { s += ",uniqueAlt=" + fmt.Sprint(cs.uniqueAlt) }
	if cs.conflictingAlts != nil { s += ",conflictingAlts=" + cs.conflictingAlts.String() }
	if cs.dipsIntoOuterContext { s += ",dipsIntoOuterContext" }
	if cs.readOnly { s += ",readOnly" }
	return s
}

// NewOrderedATNConfigSet creates a config set for lexers (uses standard ATNConfig Equals/Hash).
func NewOrderedATNConfigSet() *ATNConfigSet {
	cs := atnConfigSetPool.Get().(*ATNConfigSet)
	// Initialize for ordered set (lexer)
	cs.fullCtx = false // Lexers typically don't use fullCtx for this set type
	cs.cachedHash = -1
	cs.uniqueAlt = ATNInvalidAltNumber
	cs.readOnly = false
    cs.dipsIntoOuterContext = false
	cs.hasSemanticContext = false


	if cs.configs == nil {
	    cs.configs = make([]*ATNConfig, 0)
    } else {
        cs.configs = cs.configs[:0]
    }
	// Uses standard ATNConfig comparator (aConfEqInst)
	cs.configLookup = NewJStore[*ATNConfig, Comparator[*ATNConfig]](aConfEqInst, ATNConfigCollection, "NewOrderedATNConfigSet()")
	cs.conflictingAlts = nil
	return cs
}

[end of runtime/Go/antlr/v4/atn_config_set.go]
