package pkg

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"time"

	"m7s.live/m7s/v5/pkg/util"
)

const TraceLevel = slog.Level(-8)

var (
	ErrAutoStop     = errors.New("auto stop")
	ErrCallbackTask = errors.New("callback task")
	EmptyStart      = func() error { return nil }
	EmptyDispose    = func() {}
)

type (
	ITask interface {
		init(ctx context.Context)
		getParent() *MarcoTask
		getTask() *Task
		Stop(error)
		StopReason() error
		start() (reflect.Value, error)
		dispose()
		IsStopped() bool
	}
	IChannelTask interface {
		tick(reflect.Value)
	}
	TaskStarter interface {
		Start() error
	}
	TaskDisposal interface {
		Dispose()
	}
	Task struct {
		ID        uint32
		StartTime time.Time
		*slog.Logger
		context.Context
		context.CancelCauseFunc
		startHandler                               func() error
		afterStartListeners, afterDisposeListeners []func()
		disposeHandler                             func()
		Description                                map[string]any
		startup, shutdown                          *util.Promise
		parent                                     *MarcoTask
		parentCtx                                  context.Context
	}
)

func (task *Task) getTask() *Task {
	return task
}

func (task *Task) getParent() *MarcoTask {
	return task.parent
}

func (task *Task) GetKey() uint32 {
	return task.ID
}

func (task *Task) WaitStarted() error {
	return task.startup.Await()
}

func (task *Task) WaitStopped() error {
	return task.shutdown.Await()
}

func (task *Task) Trace(msg string, fields ...any) {
	task.Log(task.Context, TraceLevel, msg, fields...)
}

func (task *Task) IsStopped() bool {
	return task.Err() != nil
}

func (task *Task) StopReason() error {
	return context.Cause(task.Context)
}

func (task *Task) Stop(err error) {
	if task.CancelCauseFunc != nil && !task.IsStopped() {
		if task.Logger != nil {
			task.Debug("task stop", "reason", err.Error(), "elapsed", time.Since(task.StartTime), "taskId", task.ID)
		}
		task.CancelCauseFunc(err)
	}
}

func (task *Task) OnStart(listener func()) {
	task.afterStartListeners = append(task.afterStartListeners, listener)
}

func (task *Task) OnDispose(listener func()) {
	task.afterDisposeListeners = append(task.afterDisposeListeners, listener)
}

func (task *Task) start() (signal reflect.Value, err error) {
	task.StartTime = time.Now()
	err = task.startHandler()
	if task.Logger != nil {
		task.Debug("task start", "taskId", task.ID)
	}
	task.startup.Fulfill(err)
	signal = reflect.ValueOf(task.Done())
	for _, listener := range task.afterStartListeners {
		listener()
	}
	return
}

func (task *Task) dispose() {
	reason := task.StopReason()
	if task.Logger != nil {
		task.Debug("task dispose", "reason", reason, "taskId", task.ID)
	}
	task.disposeHandler()
	task.shutdown.Fulfill(reason)
	for _, listener := range task.afterDisposeListeners {
		listener()
	}
}

func (task *Task) init(ctx context.Context) {
	task.parentCtx = ctx
	task.Context, task.CancelCauseFunc = context.WithCancelCause(ctx)
	task.startup = util.NewPromise(task.Context)
	task.shutdown = util.NewPromise(task.Context)
}
