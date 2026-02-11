package antlr

// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

// Collectable is an interface that a struct should implement if it is to be
// usable as a key in legacy collections.
type Collectable[T any] interface {
	Hash() int
	Equals(other Collectable[T]) bool
}

type Comparator[T any] interface {
	Hash1(o T) int
	Equals2(T, T) bool
}
