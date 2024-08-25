package task

import (
	. "m7s.live/m7s/v5/pkg/util"
)

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
		m.Collection.Add(ctx)
	})
	ctx.OnDispose(func() {
		m.Remove(ctx)
	})
	return m.AddTask(ctx, opt...)
}
