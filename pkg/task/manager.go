package task

import (
	. "m7s.live/m7s/v5/pkg/util"
)

type ManagerItem[K comparable] interface {
	ITask
	GetKey() K
}

type Manager[K comparable, T ManagerItem[K]] struct {
	MarcoLongTask
	Collection[K, T]
}

func (m *Manager[K, T]) Add(ctx T) {
	ctx.OnStart(func() {
		m.Collection.Add(ctx)
	})
	ctx.OnDispose(func() {
		m.Remove(ctx)
	})
	m.AddTask(ctx)
}
