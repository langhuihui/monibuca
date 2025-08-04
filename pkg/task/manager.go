package task

import (
	"errors"
	"fmt"

	. "m7s.live/v5/pkg/util"
)

var ErrExist = errors.New("exist")

type ExistTaskError struct {
	Task ITask
}

func (e ExistTaskError) Error() string {
	return fmt.Sprintf("%v exist", e.Task.getKey())
}

type ManagerItem[K comparable] interface {
	ITask
	GetKey() K
}

type Manager[K comparable, T ManagerItem[K]] struct {
	Work
	Collection[K, T]
}

func (m *Manager[K, T]) Add(ctx T, opt ...any) *Task {
	ctx.OnStart(func() {
		if !m.Collection.AddUnique(ctx) {
			ctx.Stop(ErrExist)
			return
		}
		m.Debug("add", "key", ctx.GetKey(), "count", m.Length)
	})
	ctx.OnDispose(func() {
		m.Remove(ctx)
		m.Debug("remove", "key", ctx.GetKey(), "count", m.Length)
	})
	opt = append(opt, 1)
	return m.AddTask(ctx, opt...)
}

func (m *Manager[K, T]) SafeHas(key K) (ok bool) {
	if m.L == nil {
		m.Call(func() {
			ok = m.Collection.Has(key)
		})
		return ok
	}
	return m.Collection.Has(key)
}

// SafeGet 用于不同协程获取元素，防止并发请求
func (m *Manager[K, T]) SafeGet(key K) (item T, ok bool) {
	if m.L == nil {
		m.Call(func() {
			item, ok = m.Collection.Get(key)
		})
	} else {
		item, ok = m.Collection.Get(key)
	}
	return
}

// SafeRange 用于不同协程获取元素，防止并发请求
func (m *Manager[K, T]) SafeRange(f func(T) bool) {
	if m.L == nil {
		m.Call(func() {
			m.Collection.Range(f)
		})
	} else {
		m.Collection.Range(f)
	}
}

// SafeFind 用于不同协程获取元素，防止并发请求
func (m *Manager[K, T]) SafeFind(f func(T) bool) (item T, ok bool) {
	if m.L == nil {
		m.Call(func() {
			item, ok = m.Collection.Find(f)
		})
	} else {
		item, ok = m.Collection.Find(f)
	}
	return
}
