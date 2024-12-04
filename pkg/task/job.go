package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"m7s.live/v5/pkg/util"
)

var idG atomic.Uint32
var sourceFilePathPrefix string

func init() {
	if _, file, _, ok := runtime.Caller(0); ok {
		sourceFilePathPrefix = strings.TrimSuffix(file, "pkg/task/job.go")
	}
}

func GetNextTaskID() uint32 {
	return idG.Add(1)
}

// Job include tasks
type Job struct {
	Task
	cases                 []reflect.SelectCase
	addSub                chan ITask
	children              []ITask
	lazyRun               sync.Once
	eventLoopLock         sync.Mutex
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
	if blocked := mt.blocked; blocked != nil {
		blocked.Stop(mt.StopReason())
	}
	mt.addSub <- nil
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
		if child.GetTaskType() != TASK_TYPE_CALL || child.GetOwnerType() != "CallBack" {
			mt.onDescendantsDispose(child)
		}
		child.dispose()
	}
}

func (mt *Job) RangeSubTask(callback func(task ITask) bool) {
	for _, task := range mt.children {
		callback(task)
	}
}

func (mt *Job) AddDependTask(t ITask, opt ...any) (task *Task) {
	mt.Depend(t)
	return mt.AddTask(t, opt...)
}

func (mt *Job) AddTask(t ITask, opt ...any) (task *Task) {
	if task = t.GetTask(); t != task.handler { // first add
		for _, o := range opt {
			switch v := o.(type) {
			case context.Context:
				task.parentCtx = v
			case Description:
				task.SetDescriptions(v)
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
	_, file, line, ok := runtime.Caller(1)

	if ok {
		task.StartReason = fmt.Sprintf("%s:%d", strings.TrimPrefix(file, sourceFilePathPrefix), line)
	}

	mt.lazyRun.Do(func() {
		if mt.eventLoopLock.TryLock() {
			defer mt.eventLoopLock.Unlock()
			if mt.parent != nil && mt.Context == nil {
				mt.parent.AddTask(mt.handler) // second add, lazy start
			}
			mt.childrenDisposed = make(chan struct{})
			mt.addSub = make(chan ITask, 20)
			go mt.run()
		}
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
	if len(mt.addSub) > 10 {
		if mt.Logger != nil {
			mt.Warn("task wait list too many", "count", len(mt.addSub))
		}
	}
	mt.addSub <- t
	return
}

func (mt *Job) Call(callback func() error, args ...any) {
	mt.Post(callback, args...).WaitStarted()
}

func (mt *Job) Post(callback func() error, args ...any) *Task {
	task := CreateTaskByCallBack(callback, nil)
	if len(args) > 0 {
		task.SetDescription(OwnerTypeKey, args[0])
	}
	return mt.AddTask(task)
}

func (mt *Job) run() {
	mt.cases = []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mt.addSub)}}
	defer func() {
		err := recover()
		if err != nil {
			if mt.Logger != nil {
				mt.Logger.Error("job panic", "err", err, "stack", string(debug.Stack()))
			}
			if !ThrowPanic {
				mt.Stop(errors.Join(err.(error), ErrPanic))
			} else {
				panic(err)
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
		if chosen, rev, ok := reflect.Select(mt.cases); chosen == 0 {
			if rev.IsNil() {
				return
			}
			if mt.blocked = rev.Interface().(ITask); mt.blocked.getParent() != mt || mt.blocked.start() {
				mt.children = append(mt.children, mt.blocked)
				mt.cases = append(mt.cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mt.blocked.GetSignal())})
			}
		} else {
			taskIndex := chosen - 1
			mt.blocked = mt.children[taskIndex]
			switch tt := mt.blocked.(type) {
			case IChannelTask:
				tt.Tick(rev.Interface())
				if tt.IsStopped() {
					mt.onChildDispose(mt.blocked)
				}
			}
			if !ok {
				if mt.onChildDispose(mt.blocked); mt.blocked.checkRetry(mt.blocked.StopReason()) {
					if mt.blocked.reset(); mt.blocked.start() {
						mt.cases[chosen].Chan = reflect.ValueOf(mt.blocked.GetSignal())
						continue
					}
				}
				mt.children = slices.Delete(mt.children, taskIndex, taskIndex+1)
				mt.cases = slices.Delete(mt.cases, chosen, chosen+1)
			}
		}
		if !mt.handler.keepalive() && len(mt.children) == 0 {
			mt.Stop(ErrAutoStop)
		}
	}
}
