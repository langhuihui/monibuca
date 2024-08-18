package util

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"
)

const TraceLevel = slog.Level(-8)

var (
	ErrAutoStop     = errors.New("auto stop")
	ErrCallbackTask = errors.New("callback")
	ErrRetryRunOut  = errors.New("retry out")
	ErrTaskComplete = errors.New("complete")
	EmptyStart      = func() error { return nil }
	EmptyDispose    = func() {}
)

type (
	ITask interface {
		initTask(context.Context, ITask)
		getParent() *MarcoTask
		GetTask() *Task
		getSignal() reflect.Value
		Stop(error)
		StopReason() error
		start() error
		dispose()
		IsStopped() bool
		GetTaskType() string
		GetTaskTypeID() byte
		GetOwnerType() string
		SetRetry(maxRetry int, retryInterval time.Duration)
		OnStart(func())
		OnDispose(func())
	}
	IMarcoTask interface {
		RangeSubTask(func(yield ITask) bool)
		OnTaskAdded(func(ITask))
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
	TaskBlock interface {
		Run() error
	}
	TaskGo interface {
		Go() error
	}
	RetryConfig struct {
		MaxRetry      int
		RetryCount    int
		RetryInterval time.Duration
	}
	Task struct {
		ID        uint32
		StartTime time.Time
		*slog.Logger
		context.Context
		context.CancelCauseFunc
		handler                                    ITask
		retry                                      RetryConfig
		startHandler                               func() error
		afterStartListeners, afterDisposeListeners []func()
		disposeHandler                             func()
		Description                                map[string]any
		startup, shutdown                          *Promise
		parent                                     *MarcoTask
		parentCtx                                  context.Context
		needRetry                                  bool
	}
)

func (task *Task) SetRetry(maxRetry int, retryInterval time.Duration) {
	task.retry.MaxRetry = maxRetry
	task.retry.RetryInterval = retryInterval
}

func (task *Task) GetOwnerType() string {
	return reflect.TypeOf(task.handler).Elem().Name()
}

func (*Task) GetTaskType() string {
	return "base"
}

func (*Task) GetTaskTypeID() byte {
	return 0
}

func (task *Task) GetTask() *Task {
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

func (task *Task) WaitStopped() (err error) {
	err = task.WaitStarted()
	if err != nil {
		return err
	}
	if task.shutdown == nil {
		return task.StopReason()
	}
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

func (task *Task) StopReasonIs(err error) bool {
	return errors.Is(err, task.StopReason())
}

func (task *Task) Stop(err error) {
	if err == nil {
		panic("task stop with nil error")
	}
	if task.CancelCauseFunc != nil {
		if task.Logger != nil {
			task.Debug("task stop", "reason", err, "elapsed", time.Since(task.StartTime), "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType())
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

func (task *Task) getSignal() reflect.Value {
	return reflect.ValueOf(task.Done())
}

func (task *Task) checkRetry(err error) (bool, error) {
	if task.retry.MaxRetry < 0 || task.retry.RetryCount < task.retry.MaxRetry {
		task.retry.RetryCount++
		if task.Logger != nil {
			task.Warn(fmt.Sprintf("retry %d/%d", task.retry.RetryCount, task.retry.MaxRetry))
		}
		if delta := time.Since(task.StartTime); delta < task.retry.RetryInterval {
			time.Sleep(task.retry.RetryInterval - delta)
		}
		return true, err
	} else {
		if task.retry.MaxRetry > 0 {
			if task.Logger != nil {
				task.Warn(fmt.Sprintf("max retry %d failed", task.retry.MaxRetry))
			}
			return false, errors.Join(err, ErrRetryRunOut)
		}
	}
	return false, err
}

func (task *Task) start() (err error) {
	task.StartTime = time.Now()
	if task.Logger != nil {
		task.Debug("task start", "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType())
	}
	for {
		err = task.startHandler()
		if err == nil {
			task.ResetRetryCount()
			if runHandler, ok := task.handler.(TaskBlock); ok {
				err = runHandler.Run()
				if err == nil {
					err = ErrTaskComplete
					task.Stop(err)
					task.dispose()
				}
			}
		}
		if task.IsStopped() {
			return task.StopReason()
		}
		task.needRetry, err = task.checkRetry(err)
		if task.needRetry {
			task.Stop(err)
			task.dispose()
		} else {
			break
		}
	}
	task.startup.Fulfill(err)
	for _, listener := range task.afterStartListeners {
		listener()
	}
	if goHandler, ok := task.handler.(TaskGo); ok {
		go task.run(goHandler.Go)
	}
	return
}

func (task *Task) dispose() {
	reason := task.StopReason()
	if task.Logger != nil {
		task.Debug("task dispose", "reason", reason, "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType())
	}
	task.disposeHandler()
	task.shutdown.Fulfill(reason)
	for _, listener := range task.afterDisposeListeners {
		listener()
	}
	if !errors.Is(reason, ErrTaskComplete) && task.needRetry {
		task.Context, task.CancelCauseFunc = context.WithCancelCause(task.parentCtx)
		task.startup = NewPromise(task.Context)
		task.shutdown = NewPromise(context.Background())
		parent := task.parent
		task.parent = nil
		parent.AddTask(task.handler)
	}
}

func (task *Task) initTask(ctx context.Context, iTask ITask) {
	task.parentCtx = ctx
	task.Context, task.CancelCauseFunc = context.WithCancelCause(ctx)
	task.startup = NewPromise(task.Context)
	task.shutdown = NewPromise(context.Background())
	task.handler = iTask
	if v, ok := iTask.(TaskStarter); ok {
		task.startHandler = v.Start
	}
	if v, ok := iTask.(TaskDisposal); ok {
		task.disposeHandler = v.Dispose
	}
}

func (task *Task) ResetRetryCount() {
	task.retry.RetryCount = 0
}

func (task *Task) run(handler func() error) {
	var err error
	err = handler()
	if err == nil {
		task.needRetry = false
		task.Stop(ErrTaskComplete)
	} else {
		if task.needRetry, err = task.checkRetry(err); !task.needRetry {
			task.Stop(errors.Join(err, ErrRetryRunOut))
		} else {
			task.Stop(err)
		}
	}
}
