package sync

import (
	"sort"
	"sync"
)

type syncArrayList[V any] struct {
	sync.RWMutex
	l []V
}

func NewArrayList[V any]() List[V] {
	return &syncArrayList[V]{}
}

func (l *syncArrayList[V]) Add(item V) {
	l.Lock()
	defer l.Unlock()
	l.l = append(l.l, item)
}

func (l *syncArrayList[V]) RemoveAll(filter func(item V) bool) (removed bool) {
	l.Lock()
	defer l.Unlock()
	removed = false
	newList := make([]V, 0)
	for _, v := range l.l {
		if !filter(v) {
			newList = append(newList, v)
		} else {
			removed = true
		}
	}
	l.l = newList
	return
}

func (l *syncArrayList[V]) List() []V {
	l.RLock()
	defer l.RUnlock()
	res := make([]V, len(l.l))
	copy(res, l.l)
	return res
}

func (l *syncArrayList[V]) Size() int {
	l.RLock()
	defer l.RUnlock()
	return len(l.l)
}

func (l *syncArrayList[V]) Sort(less func(i1, i2 V) bool) {
	l.Lock()
	defer l.Unlock()
	sort.Slice(l.l, func(i, j int) bool {
		return less(l.l[i], l.l[j])
	})
}
