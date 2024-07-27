package sync

import (
	"sync"
	"time"
)

type cacheItem[V any] struct {
	Obj     V
	Expires time.Time
}

type cacheMap[K comparable, V any] struct {
	sync.Mutex
	m       map[K]cacheItem[V]
	expires time.Duration
	cleaner bool
}

func NewCacheMap[K comparable, V any](expires time.Duration) Map[K, V] {
	if expires == 0 {
		return NewVoidMap[K, V]()
	}
	return &cacheMap[K, V]{
		m:       make(map[K]cacheItem[V]),
		expires: expires,
		cleaner: false,
	}
}
func (c *cacheMap[K, V]) Get(key K) (v V, ok bool) {
	c.Lock()
	defer c.Unlock()
	if cache, ok := c.m[key]; ok && cache.Expires.After(time.Now()) {
		return cache.Obj, ok
	}
	return
}

func (c *cacheMap[K, V]) Put(key K, obj V) {
	c.Lock()
	defer c.Unlock()
	c.m[key] = cacheItem[V]{Obj: obj, Expires: time.Now().Add(c.expires)}
	if !c.cleaner {
		c.cleaner = true
		go c.scheduleCleaner()
	}
}

func (c *cacheMap[K, V]) Delete(key K) (v V) {
	c.Lock()
	defer c.Unlock()
	cache := c.m[key]
	if cache.Expires.After(time.Now()) {
		return cache.Obj
	}
	return
}
func (c *cacheMap[K, V]) Len() int {
	c.clearExpired()
	return len(c.m)
}
func (c *cacheMap[K, V]) Keys() []K {
	c.clearExpired()
	c.Lock()
	defer c.Unlock()
	r := make([]K, 0, len(c.m))
	for k := range c.m {
		r = append(r, k)
	}
	return r
}
func (c *cacheMap[K, V]) Values() []V {
	c.clearExpired()
	c.Lock()
	defer c.Unlock()
	r := make([]V, 0, len(c.m))
	for _, v := range c.m {
		r = append(r, v.Obj)
	}
	return r
}

func (c *cacheMap[K, V]) Clear() {
	c.Lock()
	defer c.Unlock()
	clear(c.m)
}

func (c *cacheMap[K, V]) scheduleCleaner() {
	for {
		c.clearExpired()
		c.Lock()
		end := len(c.m) == 0
		if end {
			c.cleaner = false
		}
		c.Unlock()
		if end {
			return
		}
		time.Sleep(5 * time.Minute)
	}
}

func (c *cacheMap[K, V]) clearExpired() {
	c.Lock()
	defer c.Unlock()
	expired := []K{}
	for k, cache := range c.m {
		if cache.Expires.Before(time.Now()) {
			expired = append(expired, k)
		}
	}
	for _, k := range expired {
		delete(c.m, k)
	}
}
