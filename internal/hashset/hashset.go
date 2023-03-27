package hashset

type Set[K comparable] struct {
	val map[K]struct{}
}

func New[K comparable](preallocate int) Set[K] {
	if preallocate < 0 {
		preallocate = 0
	}
	return Set[K]{
		val: make(map[K]struct{}, preallocate),
	}
}

// Insert an item into the set.
// Returns true if the key was not already present
func (s *Set[K]) Insert(key K) bool {
	if _, ok := s.val[key]; ok {
		return false
	}
	s.val[key] = struct{}{}
	return true
}

// InsertMany items into the set.
func (s *Set[K]) InsertMany(keys []K) {
	for _, key := range keys {
		s.Insert(key)
	}
}

// Remove an item from the set.
// Returns true if the item was removed, or false if it wasn't in the set.
func (s *Set[K]) Remove(key K) bool {
	if _, ok := s.val[key]; ok {
		delete(s.val, key)
		return true
	}
	return false
}

// RemoveMany items from the set.
func (s *Set[K]) RemoveMany(keys []K) {
	for _, key := range keys {
		s.Remove(key)
	}
}

// Contains returns true if a key is found in the set.
func (s *Set[K]) Contains(key K) bool {
	_, ok := s.val[key]
	return ok
}

// Keys returns every value in the set.
func (s *Set[K]) Keys() []K {
	ret := make([]K, len(s.val))
	for key := range s.val {
		ret = append(ret, key)
	}
	return ret
}

// Size returns the number of entries in the set.
func (s *Set[K]) Size() int {
	return len(s.val)
}

// Empty returns true if the set has no values.
func (s *Set[K]) Empty() bool {
	return len(s.val) == 0
}
