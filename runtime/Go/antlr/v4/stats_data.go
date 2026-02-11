// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

// NativeCollectionStats tracks basic usage of our new native Go collections.
type NativeCollectionStats struct {
	Puts uint64
	Gets uint64
	Hits uint64
}

// NativeGoStats is the modern schema for tracking ANTLR4 Go runtime performance.
type NativeGoStats struct {
	IDsAssigned       uint64
	BridgeReconstructions uint64
	ContextRegistrySize uint64
	ContextRegistryCap  uint64
	SemanticRegistrySize uint64
	
	ATNConfigSet    NativeCollectionStats
	MergeCache      NativeCollectionStats
	ContextCache    NativeCollectionStats
	DFACache        NativeCollectionStats
}

var NativeStats NativeGoStats
