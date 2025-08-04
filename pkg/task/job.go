package task

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
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
	children                    sync.Map
	descendantsDisposeListeners []func(ITask)
	descendantsStartListeners   []func(ITask)
	blocked                     ITask
	eventLoop                   EventLoop
	Size                        atomic.Int32
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

func (mt *Job) EventLoopRunning() bool {
	return mt.eventLoop.running.Load()
}

func (mt *Job) waitChildrenDispose(stopReason error) {
	mt.eventLoop.active(mt)
	mt.children.Range(func(key, value any) bool {
		child := value.(ITask)
		child.Stop(stopReason)
		child.WaitStopped()
		return true
	})
}

func (mt *Job) OnDescendantsDispose(listener func(ITask)) {
	mt.descendantsDisposeListeners = append(mt.descendantsDisposeListeners, listener)
}

func (mt *Job) onDescendantsDispose(descendants ITask) {
	for _, listener := range mt.descendantsDisposeListeners {
		listener(descendants)
	}
	if mt.parent != nil {
		mt.parent.onDescendantsDispose(descendants)
	}
}

func (mt *Job) onChildDispose(child ITask) {
	mt.onDescendantsDispose(child)
	child.dispose()
}

func (mt *Job) removeChild(child ITask) {
	value, loaded := mt.children.LoadAndDelete(child.getKey())
	if loaded {
		if value != child {
			panic("remove child")
		}
		remains := mt.Size.Add(-1)
		mt.Debug("remove child", "id", child.GetTaskID(), "remains", remains)
	}
}

func (mt *Job) OnDescendantsStart(listener func(ITask)) {
	mt.descendantsStartListeners = append(mt.descendantsStartListeners, listener)
}

func (mt *Job) onDescendantsStart(descendants ITask) {
	for _, listener := range mt.descendantsStartListeners {
		listener(descendants)
	}
	if mt.parent != nil {
		mt.parent.onDescendantsStart(descendants)
	}
}

func (mt *Job) onChildStart(child ITask) {
	mt.onDescendantsStart(child)
}

func (mt *Job) RangeSubTask(callback func(task ITask) bool) {
	mt.children.Range(func(key, value any) bool {
		callback(value.(ITask))
		return true
	})
}

func (mt *Job) AddDependTask(t ITask, opt ...any) (task *Task) {
	t.Using(mt)
	opt = append(opt, 1)
	return mt.AddTask(t, opt...)
}

func (mt *Job) initContext(task *Task, opt ...any) {
	callDepth := 2
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
		case int:
			callDepth += v
		}
	}
	_, file, line, ok := runtime.Caller(callDepth)
	if ok {
		task.StartReason = fmt.Sprintf("%s:%d", strings.TrimPrefix(file, sourceFilePathPrefix), line)
	}
	task.parent = mt
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
	if task.Logger == nil {
		task.Logger = mt.Logger
	}
}

func (mt *Job) AddTask(t ITask, opt ...any) (task *Task) {
	task = t.GetTask()
	task.handler = t
	mt.initContext(task, opt...)
	if mt.IsStopped() {
		task.startup.Reject(mt.StopReason())
		return
	}
	actual, loaded := mt.children.LoadOrStore(t.getKey(), t)
	if loaded {
		task.startup.Reject(ExistTaskError{
			Task: actual.(ITask),
		})
		return
	}
	var err error
	defer func() {
		if err != nil {
			mt.children.Delete(t.getKey())
			task.startup.Reject(err)
		}
	}()
	if err = mt.eventLoop.add(mt, t); err != nil {
		return
	}
	if mt.IsStopped() {
		err = mt.StopReason()
		return
	}
	remains := mt.Size.Add(1)
	mt.Debug("child added", "id", task.ID, "remains", remains)
	return
}

func (mt *Job) Call(callback func()) {
	if mt.Size.Load() <= 0 {
		callback()
		return
	}
	ctx, cancel := context.WithCancel(mt)
	_ = mt.eventLoop.add(mt, func() { callback(); cancel() })
	<-ctx.Done()
}
