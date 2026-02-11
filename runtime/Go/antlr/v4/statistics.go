//go:build antlr.stats

package antlr

import (
	"fmt"
	"sync/atomic"
)

// This file allows the user to collect statistics about the runtime of the ANTLR runtime. It is not enabled by default
// and so incurs no time penalty. To enable it, you must build the runtime with the antlr.stats build tag.
//

// Tells various components to collect statistics - because it is only true when this file is included, it will
// allow the compiler to completely eliminate all the code that is only used when collecting statistics.
const collectStats = true

// goRunStats is a collection of all the various data the ANTLR runtime has collected about a particular run.
type goRunStats struct {
	topN int
}

var (
	Statistics = &goRunStats{
		topN: 10,
	}
)

func (s *goRunStats) AddIDAssignment() {
	atomic.AddUint64(&NativeStats.IDsAssigned, 1)
}

func (s *goRunStats) AddBridgeReconstruction() {
	atomic.AddUint64(&NativeStats.BridgeReconstructions, 1)
}

func (s *goRunStats) AddContextCacheHit() {
	atomic.AddUint64(&NativeStats.ContextCache.Hits, 1)
}

func (s *goRunStats) AddMergeCacheHit() {
	atomic.AddUint64(&NativeStats.MergeCache.Hits, 1)
}

func (s *goRunStats) Report() {
	fmt.Printf("\n--- ANTLR4 Native Go Statistics ---\n")
	fmt.Printf("IDs Assigned:           %d\n", atomic.LoadUint64(&NativeStats.IDsAssigned))
	fmt.Printf("Bridge Reconstructions: %d\n", atomic.LoadUint64(&NativeStats.BridgeReconstructions))
	fmt.Printf("Context Registry Size:  %d\n", atomic.LoadUint64(&NativeStats.ContextRegistrySize))
	fmt.Printf("Context Cache Hits:     %d\n", atomic.LoadUint64(&NativeStats.ContextCache.Hits))
	fmt.Printf("Merge Cache Hits:       %d\n", atomic.LoadUint64(&NativeStats.MergeCache.Hits))
	fmt.Printf("------------------------------------\n")
}

func (s *goRunStats) Analyze() {
	// Compatibility stub
}

func (s *goRunStats) Reset() {
	NativeStats = NativeGoStats{}
}

func (s *goRunStats) Configure(_ ...any) error {
	return nil
}
