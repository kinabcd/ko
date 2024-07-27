package sync

import (
	"sync"
)

type Map[K comparable, V any] interface {
	Put(k K, v V)
	Get(k K) (V, bool)
	Delete(k K) V
	Len() int
	Keys() []K
	Values() []V
	Clear()
}

type syncMap[K comparable, V any] struct {
	sync.RWMutex
	m map[K]V
}

func NewMap[K comparable, V any]() Map[K, V] {
	return &syncMap[K, V]{
		m: map[K]V{},
	}
}

func (o *syncMap[K, V]) Put(k K, v V) {
	o.Lock()
	defer o.Unlock()
	o.m[k] = v
}

func (o *syncMap[K, V]) Get(k K) (V, bool) {
	o.RLock()
	defer o.RUnlock()
	v, ok := o.m[k]
	return v, ok
}

func (o *syncMap[K, V]) Delete(k K) V {
	o.Lock()
	defer o.Unlock()
	v := o.m[k]
	delete(o.m, k)
	return v
}

func (o *syncMap[K, V]) Len() int {
	return len(o.m)
}

func (o *syncMap[K, V]) Keys() []K {
	o.RLock()
	defer o.RUnlock()
	r := make([]K, 0, len(o.m))
	for k := range o.m {
		r = append(r, k)
	}
	return r
}
func (o *syncMap[K, V]) Values() []V {
	o.RLock()
	defer o.RUnlock()
	r := make([]V, 0, len(o.m))
	for _, v := range o.m {
		r = append(r, v)
	}
	return r
}
func (o *syncMap[K, V]) Clear() {
	o.Lock()
	defer o.Unlock()
	clear(o.m)
}
