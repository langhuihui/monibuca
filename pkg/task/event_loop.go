package task

import (
	"errors"
	"reflect"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
)

type Singleton[T comparable] struct {
	instance atomic.Value
	mux      sync.Mutex
}

func (s *Singleton[T]) Load() T {
	return s.instance.Load().(T)
}

func (s *Singleton[T]) Get(newF func() T) T {
	ch := s.instance.Load() //fast
	if ch == nil {          // slow
		s.mux.Lock()
		defer s.mux.Unlock()
		if ch = s.instance.Load(); ch == nil {
			ch = newF()
			s.instance.Store(ch)
		}
	}
	return ch.(T)
}

type EventLoop struct {
	cases    []reflect.SelectCase
	children []ITask
	addSub   Singleton[chan any]
	running  atomic.Bool
}

func (e *EventLoop) getInput() chan any {
	return e.addSub.Get(func() chan any {
		return make(chan any, 20)
	})
}

func (e *EventLoop) active(mt *Job) {
	if mt.parent != nil {
		mt.parent.eventLoop.active(mt.parent)
	}
	if e.running.CompareAndSwap(false, true) {
		go e.run(mt)
	}
}

func (e *EventLoop) add(mt *Job, sub any) (err error) {
	shouldActive := true
	switch sub.(type) {
	case TaskStarter, TaskBlock, TaskGo:
	case IJob:
		shouldActive = false
	}
	select {
	case e.getInput() <- sub:
		if shouldActive || mt.IsStopped() {
			e.active(mt)
		}
		return nil
	default:
		return ErrTooManyChildren
	}
}

func (e *EventLoop) run(mt *Job) {
	mt.Debug("event loop start", "jobId", mt.GetTaskID(), "type", mt.GetOwnerType())
	ch := e.getInput()
	e.cases = []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch)}}
	defer func() {
		err := recover()
		if err != nil {
			mt.Error("job panic", "err", err, "stack", string(debug.Stack()))
			if !ThrowPanic {
				mt.Stop(errors.Join(err.(error), ErrPanic))
			} else {
				panic(err)
			}
		}
		mt.Debug("event loop exit", "jobId", mt.GetTaskID(), "type", mt.GetOwnerType())
		if !mt.handler.keepalive() {
			if mt.blocked != nil {
				mt.Stop(errors.Join(mt.blocked.StopReason(), ErrAutoStop))
			} else {
				mt.Stop(ErrAutoStop)
			}
		}
		mt.blocked = nil
	}()

	// Main event loop - only exit when no more events AND no children
	for {
		if len(ch) == 0 && len(e.children) == 0 {
			if e.running.CompareAndSwap(true, false) {
				if len(ch) > 0 { // if add before running set to false
					e.active(mt)
				}
				return
			}
		}
		mt.blocked = nil
		if chosen, rev, ok := reflect.Select(e.cases); chosen == 0 {
			if !ok {
				mt.Debug("job addSub channel closed, exiting", "taskId", mt.GetTaskID())
				mt.Stop(ErrAutoStop)
				return
			}
			switch v := rev.Interface().(type) {
			case func():
				v()
			case ITask:
				if len(e.cases) >= 65535 {
					mt.Warn("task children too many, may cause performance issue", "count", len(e.cases), "taskId", mt.GetTaskID(), "taskType", mt.GetTaskType(), "ownerType", mt.GetOwnerType())
					v.Stop(ErrTooManyChildren)
					continue
				}
				if mt.blocked = v; v.start() {
					e.cases = append(e.cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(v.GetSignal())})
					e.children = append(e.children, v)
					mt.onChildStart(v)
				} else {
					mt.removeChild(v)
				}
			}
		} else {
			taskIndex := chosen - 1
			child := e.children[taskIndex]
			mt.blocked = child
			switch tt := mt.blocked.(type) {
			case IChannelTask:
				if tt.IsStopped() {
					switch ttt := tt.(type) {
					case ITickTask:
						ttt.GetTicker().Stop()
					}
					mt.onChildDispose(child)
					mt.removeChild(child)
					e.children = slices.Delete(e.children, taskIndex, taskIndex+1)
					e.cases = slices.Delete(e.cases, chosen, chosen+1)
				} else {
					tt.Tick(rev.Interface())
				}
			default:
				if !ok {
					if mt.onChildDispose(child); child.checkRetry(child.StopReason()) {
						if child.reset(); child.start() {
							e.cases[chosen].Chan = reflect.ValueOf(child.GetSignal())
							mt.onChildStart(child)
							continue
						}
					}
					mt.removeChild(child)
					e.children = slices.Delete(e.children, taskIndex, taskIndex+1)
					e.cases = slices.Delete(e.cases, chosen, chosen+1)
				}
			}
		}
	}
}
