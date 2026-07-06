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
	if c.L != nil {
		c.L.Lock()
		defer c.L.Unlock()
	}
	key := item.GetKey()
	for i := range c.Items {
		if c.Items[i].GetKey() == key {
			c.Items[i] = item
			if c.m != nil {
				c.m[key] = item
			}
			return false
		}
	}
	c.Items = append(c.Items, item)
	if c.Length > 100 || c.m != nil {
		if c.m == nil {
			c.m = make(map[K]T)
			for _, v := range c.Items {
				c.m[v.GetKey()] = v
			}
		}
		c.m[key] = item
	}
	c.Length++
	for _, listener := range c.addListeners {
		listener(item)
	}
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
	for i := 0; i < len(c.Items); i++ {
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

func (c *Collection[K, T]) Has(key K) bool {
	_, ok := c.Get(key)
	return ok
}

func (c *Collection[K, T]) Get(key K) (item T, ok bool) {
	if c.L != nil {
		c.L.RLock()
		defer c.L.RUnlock()
	}
	if c.m != nil {
		item, ok = c.m[key]
		return
	}
	for _, item := range c.Items {
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
	for _, item := range c.Items {
		if f(item) {
			return item, true
		}
	}
	return
}

func (c *Collection[K, T]) Search(f func(T) bool) func(yield func(item T) bool) {
	if c.L != nil {
		c.L.RLock()
		defer c.L.RUnlock()
	}
	return func(yield func(item T) bool) {
		for _, item := range c.Items {
			if f(item) {
				if !yield(item) {
					break
				}
			}
		}
	}
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

// LoadOrStore 返回键的现有值(如果存在)，否则存储并返回给定的值。
// loaded 结果表示是否找到了值，如果为 true 则表示找到了现有值，false 表示存储了新值。
func (c *Collection[K, T]) LoadOrStore(item T) (actual T, loaded bool) {
	key := item.GetKey()
	if c.L != nil {
		c.L.Lock()
		defer c.L.Unlock()
	}

	// 先尝试获取现有值
	if c.m != nil {
		actual, loaded = c.m[key]
	} else {
		for _, v := range c.Items {
			if v.GetKey() == key {
				actual = v
				loaded = true
				break
			}
		}
	}

	// 如果没有找到现有值，则存储新值
	if !loaded {
		c.Items = append(c.Items, item)
		if c.Length > 100 || c.m != nil {
			if c.m == nil {
				c.m = make(map[K]T)
				for _, v := range c.Items {
					c.m[v.GetKey()] = v
				}
			}
			c.m[key] = item
		}
		c.Length++
		actual = item

		// 触发添加监听器
		for _, listener := range c.addListeners {
			listener(item)
		}
	}

	return actual, loaded
}
