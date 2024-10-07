package task

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"

	"m7s.live/m7s/v5/pkg/util"
)

var idG atomic.Uint32

func GetNextTaskID() uint32 {
	return idG.Add(1)
}

// Job include tasks
type Job struct {
	Task
	addSub                chan ITask
	children              []ITask
	lazyRun               sync.Once
	childrenDisposed      chan struct{}
	childDisposeListeners []func(ITask)
	blocked               ITask
}

func (*Job) GetTaskType() TaskType {
	return TASK_TYPE_JOB
}

func (mt *Job) getJob() *Job {
	return mt
}

func (mt *Job) Blocked() ITask {
	return mt.blocked
}

func (mt *Job) waitChildrenDispose() {
	close(mt.addSub)
	<-mt.childrenDisposed
}

func (mt *Job) OnChildDispose(listener func(ITask)) {
	mt.childDisposeListeners = append(mt.childDisposeListeners, listener)
}

func (mt *Job) onDescendantsDispose(descendants ITask) {
	for _, listener := range mt.childDisposeListeners {
		listener(descendants)
	}
	if mt.parent != nil {
		mt.parent.onDescendantsDispose(descendants)
	}
}

func (mt *Job) onChildDispose(child ITask) {
	if child.getParent() == mt {
		mt.onDescendantsDispose(child)
		child.dispose()
	}
}

func (mt *Job) dispose() {
	if mt.childrenDisposed != nil {
		mt.OnBeforeDispose(mt.waitChildrenDispose)
	}
	mt.Task.dispose()
}

func (mt *Job) RangeSubTask(callback func(task ITask) bool) {
	for _, task := range mt.children {
		callback(task)
	}
}

func (mt *Job) AddTask(t ITask, opt ...any) (task *Task) {
	if task = t.GetTask(); t != task.handler { // first add
		for _, o := range opt {
			switch v := o.(type) {
			case context.Context:
				task.parentCtx = v
			case Description:
				task.Description = v
			case RetryConfig:
				task.retry = v
			case *slog.Logger:
				task.Logger = v
			}
		}
		task.parent = mt
		task.handler = t
		switch t.(type) {
		case TaskStarter, TaskBlock, TaskGo:
			// need start now
		case IJob:
			// lazy start
			return
		}
	}

	mt.lazyRun.Do(func() {
		if mt.parent != nil && mt.Context == nil {
			mt.parent.AddTask(mt.handler) // second add, lazy start
		}
		mt.childrenDisposed = make(chan struct{})
		mt.addSub = make(chan ITask, 10)
		go mt.run()
	})
	if task.Context == nil {
		if task.parentCtx == nil {
			task.parentCtx = mt.Context
		}
		task.level = mt.level + 1
		if task.ID == 0 {
			task.ID = GetNextTaskID()
		}
		task.Context, task.CancelCauseFunc = context.WithCancelCause(task.parentCtx)
		task.startup = util.NewPromise(task.Context)
		task.shutdown = util.NewPromise(context.Background())
		task.handler = t
		if task.Logger == nil {
			task.Logger = mt.Logger
		}
	}
	if mt.IsStopped() {
		task.startup.Reject(mt.StopReason())
		return
	}

	mt.addSub <- t
	return
}

func (mt *Job) Call(callback func() error, args ...any) {
	mt.Post(callback, args...).WaitStarted()
}

func (mt *Job) Post(callback func() error, args ...any) *Task {
	task := CreateTaskByCallBack(callback, nil)
	description := make(Description)
	if len(args) > 0 {
		description[OwnerTypeKey] = args[0]
	} else {
		description = nil
	}
	return mt.AddTask(task, description)
}

func (mt *Job) run() {
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mt.addSub)}}
	defer func() {
		if !ThrowPanic {
			err := recover()
			if err != nil {
				if mt.Logger != nil {
					mt.Logger.Error("job panic", "err", err, "stack", string(debug.Stack()))
				}
				mt.Stop(errors.Join(err.(error), ErrPanic))
			}
		}
		stopReason := mt.StopReason()
		for _, task := range mt.children {
			task.Stop(stopReason)
			mt.onChildDispose(task)
		}
		mt.children = nil
		close(mt.childrenDisposed)
	}()
	for {
		mt.blocked = nil
		if chosen, rev, ok := reflect.Select(cases); chosen == 0 {
			if !ok {
				return
			}
			if mt.blocked = rev.Interface().(ITask); mt.blocked.getParent() != mt || mt.blocked.start() {
				mt.children = append(mt.children, mt.blocked)
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mt.blocked.GetSignal())})
			}
		} else {
			taskIndex := chosen - 1
			child := mt.children[taskIndex]
			switch tt := child.(type) {
			case IChannelTask:
				tt.Tick(rev.Interface())
				if tt.IsStopped() {
					mt.onChildDispose(child)
				}
			}
			if !ok {
				if mt.onChildDispose(child); child.checkRetry(child.StopReason()) {
					if child.reset(); child.start() {
						cases[chosen].Chan = reflect.ValueOf(child.GetSignal())
						continue
					}
				}
				mt.children = slices.Delete(mt.children, taskIndex, taskIndex+1)
				cases = slices.Delete(cases, chosen, chosen+1)
			}
		}
		if !mt.handler.keepalive() && len(mt.children) == 0 {
			mt.Stop(ErrAutoStop)
		}
	}
}
