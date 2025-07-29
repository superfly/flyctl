package set

import (
	"github.com/samber/lo"
	"github.com/superfly/flyctl/helpers"
)

type Set[T comparable] struct {
	values map[T]struct{}
}

// We can't use interfaces because we have generics, so we'll just lazy init on setters.
// Alternatively, we could just require that the user use a constructor, but that doesn't
// *guarantee* that we can't get a nil map.
func (s *Set[T]) ensureNotNull() {
	if s.values == nil {
		s.values = map[T]struct{}{}
	}
}

func (s *Set[T]) Set(values ...T) {
	s.ensureNotNull()
	for _, value := range values {
		s.values[value] = struct{}{}
	}
}
func (s *Set[T]) Unset(values ...T) {
	s.ensureNotNull()
	for _, value := range values {
		delete(s.values, value)
	}
}
func (s *Set[T]) Has(value T) bool {
	_, ok := s.values[value]
	return ok
}
func (s *Set[T]) HasAll(values ...T) bool {
	for _, value := range values {
		if !s.Has(value) {
			return false
		}
	}
	return true
}
func (s *Set[T]) HasAny(values ...T) bool {
	for _, value := range values {
		if s.Has(value) {
			return true
		}
	}
	return false
}
func (s *Set[T]) Values() []T {
	s.ensureNotNull()
	return lo.Keys(s.values)
}
func (s *Set[T]) Len() int {
	s.ensureNotNull()
	return len(s.values)
}
func (s *Set[T]) Clear() {
	s.Unset(s.Values()...)
}
func (s *Set[T]) Copy() Set[T] {
	return Set[T]{helpers.Clone(s.values)}
}
