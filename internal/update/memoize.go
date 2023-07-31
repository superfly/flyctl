package update

import "sync"

type memoize[T any] struct {
	val  T
	err  error
	done bool

	lock sync.Mutex
}

func (m *memoize[T]) Get(fn func() (T, error)) (T, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.done {
		return m.val, m.err
	}

	m.val, m.err = fn()
	m.done = true

	return m.val, m.err
}
