// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import "sync"

var atnConfigSetPool = sync.Pool{
	New: func() interface{} {
		return &ATNConfigSet{
			configLookup: make(map[int][]int),
			configs:      make([]ATNConfig, 0, 16),
		}
	},
}

func acquireATNConfigSet(fullCtx bool) *ATNConfigSet {
	s := atnConfigSetPool.Get().(*ATNConfigSet)
	s.Clear()
	s.fullCtx = fullCtx
	s.readOnly = false
	return s
}

func releaseATNConfigSet(s *ATNConfigSet) {
	if s == nil || s.readOnly {
		return
	}
	atnConfigSetPool.Put(s)
}

var bitSetPool = sync.Pool{
	New: func() interface{} {
		return &BitSet{}
	},
}

func acquireBitSet() *BitSet {
	b := bitSetPool.Get().(*BitSet)
	b.ClearAll()
	return b
}

func releaseBitSet(b *BitSet) {
	if b == nil {
		return
	}
	bitSetPool.Put(b)
}
