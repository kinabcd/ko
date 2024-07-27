package sync

import (
	"sync"
)

type ListMap[K comparable, V any] interface {
	Add(K, V) bool
	RemoveAll(K, func(V) bool) (bool, bool)
	Get(K) []V
	Keys() []K
	Sort(K, func(v1 V, v2 V) (less bool))
}
type syncListMap[K comparable, V any] struct {
	sync.RWMutex
	m map[K]List[V]
}

func NewListMap[K comparable, V any]() ListMap[K, V] {
	return &syncListMap[K, V]{
		m: map[K]List[V]{},
	}
}

func (lm *syncListMap[K, V]) Add(key K, item V) (newKey bool) {
	lm.Lock()
	defer lm.Unlock()
	newKey = false
	list, ok := lm.m[key]
	if !ok {
		newKey = true
		list = NewArrayList[V]()
		lm.m[key] = list
	} else {
		newKey = list.Size() == 0
	}
	list.Add(item)
	return
}

func (lm *syncListMap[K, V]) RemoveAll(key K, filter func(item V) bool) (any bool, all bool) {
	lm.Lock()
	defer lm.Unlock()
	all = false
	if l, ok := lm.m[key]; ok {
		any = l.RemoveAll(filter)
		if l.Size() == 0 {
			delete(lm.m, key)
			all = true
		}
		return
	}

	return false, true
}
func (lm *syncListMap[K, V]) Get(key K) []V {
	lm.RLock()
	defer lm.RUnlock()
	if l, ok := lm.m[key]; !ok {
		return make([]V, 0)
	} else {
		return l.List()
	}
}

func (lm *syncListMap[K, V]) Keys() []K {
	lm.RLock()
	defer lm.RUnlock()
	keys := make([]K, 0, len(lm.m))
	for k := range lm.m {
		keys = append(keys, k)
	}
	return keys
}

func (lm *syncListMap[K, V]) Sort(key K, less func(i1, i2 V) bool) {
	lm.RLock()
	l, ok := lm.m[key]
	lm.RUnlock()
	if ok {
		l.Sort(less)
	}
}
