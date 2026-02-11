package antlr

// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

import (
	"sort"
)

// Collectable is an interface that a struct should implement if it is to be
// usable as a key in these collections.
type Collectable[T any] interface {
	Hash() int
	Equals(other Collectable[T]) bool
}

type Comparator[T any] interface {
	Hash1(o T) int
	Equals2(T, T) bool
}

type CollectionSource int
type CollectionDescriptor struct {
	SybolicName string
	Description string
}

const (
	UnknownCollection CollectionSource = iota
	ATNConfigLookupCollection
	ATNStateCollection
	DFAStateCollection
	ATNConfigCollection
	PredictionContextCollection
	SemanticContextCollection
	ClosureBusyCollection
	PredictionVisitedCollection
	MergeCacheCollection
	PredictionContextCacheCollection
	AltSetCollection
	ReachSetCollection
)

var CollectionDescriptors = map[CollectionSource]CollectionDescriptor{
	UnknownCollection: {
		SybolicName: "UnknownCollection",
		Description: "Unknown collection type. Only used if the target author thought it was an unimportant collection.",
	},
	ATNConfigCollection: {
		SybolicName: "ATNConfigCollection",
		Description: "ATNConfig collection. Used to store the ATNConfigs for a particular state in the ATN." +
			"For instance, it is used to store the results of the closure() operation in the ATN.",
	},
	ATNConfigLookupCollection: {
		SybolicName: "ATNConfigLookupCollection",
		Description: "ATNConfigLookup collection. Used to store the ATNConfigs for a particular state in the ATN." +
			"This is used to prevent duplicating equivalent states in an ATNConfigurationSet.",
	},
	ATNStateCollection: {
		SybolicName: "ATNStateCollection",
		Description: "ATNState collection. This is used to store the states of the ATN.",
	},
	DFAStateCollection: {
		SybolicName: "DFAStateCollection",
		Description: "DFAState collection. This is used to store the states of the DFA.",
	},
	PredictionContextCollection: {
		SybolicName: "PredictionContextCollection",
		Description: "PredictionContext collection. This is used to store the prediction contexts of the ATN and cache computes.",
	},
	SemanticContextCollection: {
		SybolicName: "SemanticContextCollection",
		Description: "SemanticContext collection. This is used to store the semantic contexts of the ATN.",
	},
	ClosureBusyCollection: {
		SybolicName: "ClosureBusyCollection",
		Description: "ClosureBusy collection. This is used to check and prevent infinite recursion right recursive rules." +
			"It stores ATNConfigs that are currently being processed in the closure() operation.",
	},
	PredictionVisitedCollection: {
		SybolicName: "PredictionVisitedCollection",
		Description: "A map that records whether we have visited a particular context when searching through cached entries.",
	},
	MergeCacheCollection: {
		SybolicName: "MergeCacheCollection",
		Description: "A map that records whether we have already merged two particular contexts and can save effort by not repeating it.",
	},
	PredictionContextCacheCollection: {
		SybolicName: "PredictionContextCacheCollection",
		Description: "A map that records whether we have already created a particular context and can save effort by not computing it again.",
	},
	AltSetCollection: {
		SybolicName: "AltSetCollection",
		Description: "Used to eliminate duplicate alternatives in an ATN config set.",
	},
	ReachSetCollection: {
		SybolicName: "ReachSetCollection",
		Description: "Used as merge cache to prevent us needing to compute the merge of two states if we have already done it.",
	},
}

// JStore implements a container that allows the use of a struct to calculate the key
// for a collection of values akin to map. This is not meant to be a full-blown HashMap but just
// serve the needs of the ANTLR Go runtime.
type JStore[T any, C Comparator[T]] struct {
	store      map[int][]T
	len        int
	comparator Comparator[T]
}

func NewJStore[T any, C Comparator[T]](comparator Comparator[T], _ CollectionSource, _ string) *JStore[T, C] {

	if comparator == nil {
		panic("comparator cannot be nil")
	}

	s := &JStore[T, C]{
		store:      make(map[int][]T, 1),
		comparator: comparator,
	}
	return s
}

func (s *JStore[T, C]) Put(value T) (v T, exists bool) {

	kh := s.comparator.Hash1(value)

	for _, v1 := range s.store[kh] {
		if s.comparator.Equals2(value, v1) {
			return v1, true
		}
	}
	s.store[kh] = append(s.store[kh], value)
	s.len++
	return value, false
}

func (s *JStore[T, C]) Get(key T) (T, bool) {
	kh := s.comparator.Hash1(key)
	for _, v := range s.store[kh] {
		if s.comparator.Equals2(key, v) {
			return v, true
		}
	}
	return key, false
}

func (s *JStore[T, C]) Contains(key T) bool {
	_, present := s.Get(key)
	return present
}

func (s *JStore[T, C]) SortedSlice(less func(i, j T) bool) []T {
	vs := make([]T, 0, len(s.store))
	for _, v := range s.store {
		vs = append(vs, v...)
	}
	sort.Slice(vs, func(i, j int) bool {
		return less(vs[i], vs[j])
	})

	return vs
}

func (s *JStore[T, C]) Each(f func(T) bool) {
	for _, e := range s.store {
		for _, v := range e {
			f(v)
		}
	}
}

func (s *JStore[T, C]) Len() int {
	return s.len
}

func (s *JStore[T, C]) Values() []T {
	vs := make([]T, 0, len(s.store))
	for _, e := range s.store {
		vs = append(vs, e...)
	}
	return vs
}

type entry[K, V any] struct {
	key K
	val V
}

type JMap[K, V any, C Comparator[K]] struct {
	store      map[int][]*entry[K, V]
	len        int
	comparator Comparator[K]
}

func NewJMap[K, V any, C Comparator[K]](comparator Comparator[K], _ CollectionSource, _ string) *JMap[K, V, C] {
	m := &JMap[K, V, C]{
		store:      make(map[int][]*entry[K, V], 1),
		comparator: comparator,
	}
	return m
}

func (m *JMap[K, V, C]) Put(key K, val V) (V, bool) {
	kh := m.comparator.Hash1(key)

	for _, e := range m.store[kh] {
		if m.comparator.Equals2(e.key, key) {
			return e.val, true
		}
	}
	m.store[kh] = append(m.store[kh], &entry[K, V]{key, val})
	m.len++
	return val, false
}

func (m *JMap[K, V, C]) Values() []V {
	vs := make([]V, 0, len(m.store))
	for _, e := range m.store {
		for _, v := range e {
			vs = append(vs, v.val)
		}
	}
	return vs
}

func (m *JMap[K, V, C]) Get(key K) (V, bool) {
	var none V
	kh := m.comparator.Hash1(key)
	for _, e := range m.store[kh] {
		if m.comparator.Equals2(e.key, key) {
			return e.val, true
		}
	}
	return none, false
}

func (m *JMap[K, V, C]) Len() int {
	return m.len
}

func (m *JMap[K, V, C]) Delete(key K) {
	kh := m.comparator.Hash1(key)
	for i, e := range m.store[kh] {
		if m.comparator.Equals2(e.key, key) {
			m.store[kh] = append(m.store[kh][:i], m.store[kh][i+1:]...)
			m.len--
			return
		}
	}
}

func (m *JMap[K, V, C]) Clear() {
	m.store = make(map[int][]*entry[K, V])
}
