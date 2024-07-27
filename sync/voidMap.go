package sync

type voidMap[K comparable, V any] struct {
}

func NewVoidMap[K comparable, V any]() Map[K, V] {
	return &voidMap[K, V]{}
}

func (m *voidMap[K, V]) Put(k K, v V) {}
func (m *voidMap[K, V]) Get(k K) (o V, ok bool) {
	return
}
func (m *voidMap[K, V]) Delete(k K) (o V) {
	return
}
func (m *voidMap[K, V]) Len() int {
	return 0
}
func (m *voidMap[K, V]) Keys() []K {
	return []K{}
}
func (m *voidMap[K, V]) Values() []V {
	return []V{}
}
func (m *voidMap[K, V]) Clear() {}
