package sync

type List[K any] interface {
	Add(item K)
	RemoveAll(filter func(item K) bool) (removed bool)
	List() []K
	Size() int
	Sort(less func(i1, i2 K) bool)
}
