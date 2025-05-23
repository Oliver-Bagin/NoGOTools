// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package persistent

import (
	"iter"

	"github.com/tinygo-org/tinygo/x-tools/gopls/internal/util/constraints"
)

// Set is a collection of elements of type K.
//
// It uses immutable data structures internally, so that sets can be cloned in
// constant time.
//
// The zero value is a valid empty set.
type Set[K constraints.Ordered] struct {
	impl *Map[K, struct{}]
}

// Clone creates a copy of the receiver.
func (s *Set[K]) Clone() *Set[K] {
	clone := new(Set[K])
	if s.impl != nil {
		clone.impl = s.impl.Clone()
	}
	return clone
}

// Destroy destroys the set.
//
// After Destroy, the Set should not be used again.
func (s *Set[K]) Destroy() {
	if s.impl != nil {
		s.impl.Destroy()
	}
}

// Contains reports whether s contains the given key.
func (s *Set[K]) Contains(key K) bool {
	if s.impl == nil {
		return false
	}
	_, ok := s.impl.Get(key)
	return ok
}

// All returns the sequence of set elements in ascending order.
func (s *Set[K]) All() iter.Seq[K] {
	return func(yield func(K) bool) {
		if s.impl != nil {
			s.impl.root.forEach(func(k, _ any) bool {
				return yield(k.(K))
			})
		}
	}
}

// AddAll adds all elements from other to the receiver set.
func (s *Set[K]) AddAll(other *Set[K]) {
	if other.impl != nil {
		if s.impl == nil {
			s.impl = new(Map[K, struct{}])
		}
		s.impl.SetAll(other.impl)
	}
}

// Add adds an element to the set.
func (s *Set[K]) Add(key K) {
	if s.impl == nil {
		s.impl = new(Map[K, struct{}])
	}
	s.impl.Set(key, struct{}{}, nil)
}

// Remove removes an element from the set.
func (s *Set[K]) Remove(key K) {
	if s.impl != nil {
		s.impl.Delete(key)
	}
}
