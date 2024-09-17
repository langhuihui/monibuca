package util

import (
	"slices"
	"sync"
)

type Collection[K comparable, T interface{ GetKey() K }] struct {
	L               *sync.RWMutex
	Items           []T
	m               map[K]T
	Length          int
	addListeners    []func(T)
	removeListeners []func(T)
}

func (c *Collection[K, T]) OnAdd(listener func(T)) {
	c.addListeners = append(c.addListeners, listener)
}

func (c *Collection[K, T]) OnRemove(listener func(T)) {
	c.removeListeners = append(c.removeListeners, listener)
}

func (c *Collection[K, T]) Add(item T) {
	if c.L != nil {
		c.L.Lock()
		defer c.L.Unlock()
	}
	c.Items = append(c.Items, item)
	if c.Length > 100 || c.m != nil {
		if c.m == nil {
			c.m = make(map[K]T)
			for _, v := range c.Items {
				c.m[v.GetKey()] = v
			}
		}
		c.m[item.GetKey()] = item
	}
	c.Length++
	for _, listener := range c.addListeners {
		listener(item)
	}
}

func (c *Collection[K, T]) AddUnique(item T) (ok bool) {
	if _, ok = c.Get(item.GetKey()); !ok {
		c.Add(item)
	}
	return !ok
}

func (c *Collection[K, T]) Set(item T) (added bool) {
	key := item.GetKey()
	if c.m != nil {
		c.m[key] = item
	}
	for i := range c.Items {
		if c.Items[i].GetKey() == key {
			c.Items[i] = item
			return false
		}
	}
	c.Add(item)
	return true
}

func (c *Collection[K, T]) Range(f func(T) bool) {
	if c.L != nil {
		c.L.RLock()
		defer c.L.RUnlock()
	}
	for _, item := range c.Items {
		if !f(item) {
			break
		}
	}
}

func (c *Collection[K, T]) Remove(item T) bool {
	return c.RemoveByKey(item.GetKey())
}

func (c *Collection[K, T]) RemoveByKey(key K) bool {
	if c.L != nil {
		c.L.Lock()
		defer c.L.Unlock()
	}
	delete(c.m, key)
	for i := range c.Length {
		if c.Items[i].GetKey() == key {
			item := c.Items[i]
			c.Items = slices.Delete(c.Items, i, i+1)
			c.Length--
			for _, listener := range c.removeListeners {
				listener(item)
			}
			return true
		}
	}
	return false
}

// func (c *Collection[K, T]) GetOrCreate(key K) (item T, find bool) {
// 	if c.L != nil {
// 		c.L.Lock()
// 		defer c.L.Unlock()
// 	}
// 	if c.m != nil {
// 		item, find = c.m[key]
// 		return item, find
// 	}
// 	for _, item = range c.Items {
// 		if item.GetKey() == key {
// 			return item, true
// 		}
// 	}
// 	item = reflect.New(reflect.TypeOf(item).Elem()).Interface().(T)
// 	return
// }

func (c *Collection[K, T]) Get(key K) (item T, ok bool) {
	if c.L != nil {
		c.L.RLock()
		defer c.L.RUnlock()
	}
	if c.m != nil {
		item, ok = c.m[key]
		return item, ok
	}
	for _, item = range c.Items {
		if item.GetKey() == key {
			return item, true
		}
	}
	return
}

func (c *Collection[K, T]) Find(f func(T) bool) (item T, ok bool) {
	if c.L != nil {
		c.L.RLock()
		defer c.L.RUnlock()
	}
	for _, item = range c.Items {
		if f(item) {
			return item, true
		}
	}
	return
}

func (c *Collection[K, T]) GetKey() K {
	return c.Items[0].GetKey()
}

func (c *Collection[K, T]) Clear() {
	if c.L != nil {
		c.L.Lock()
		defer c.L.Unlock()
	}
	c.Items = nil
	c.m = nil
	c.Length = 0
}
