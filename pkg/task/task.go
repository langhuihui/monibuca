package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"m7s.live/m7s/v5/pkg/util"
)

const TraceLevel = slog.Level(-8)

var (
	ErrAutoStop     = errors.New("auto stop")
	ErrRetryRunOut  = errors.New("retry out")
	ErrTaskComplete = errors.New("complete")
	ErrExit         = errors.New("exit")
	ErrPanic        = errors.New("panic")
)

const (
	TASK_STATE_INIT TaskState = iota
	TASK_STATE_STARTING
	TASK_STATE_STARTED
	TASK_STATE_RUNNING
	TASK_STATE_GOING
	TASK_STATE_DISPOSING
	TASK_STATE_DISPOSED
)

const (
	TASK_TYPE_TASK TaskType = iota
	TASK_TYPE_JOB
	TASK_TYPE_Work
	TASK_TYPE_CHANNEL
	TASK_TYPE_CALL
)

type (
	TaskState byte
	TaskType  byte
	ITask     interface {
		context.Context
		keepalive() bool
		getParent() *Job
		GetParent() ITask
		GetTask() *Task
		GetTaskID() uint32
		GetSignal() any
		Stop(error)
		StopReason() error
		start() error
		dispose()
		IsStopped() bool
		GetTaskType() TaskType
		GetOwnerType() string
		SetRetry(maxRetry int, retryInterval time.Duration)
		OnStart(func())
		OnBeforeDispose(func())
		OnDispose(func())
		GetState() TaskState
		GetLevel() byte
	}
	IJob interface {
		ITask
		getJob() *Job
		AddTask(ITask, ...any) *Task
		RangeSubTask(func(yield ITask) bool)
		OnChildDispose(func(ITask))
		Blocked() bool
		Call(func() error)
		Post(func() error) *Task
	}
	IChannelTask interface {
		Tick(any)
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
	Description = map[string]any
	Task        struct {
		ID        uint32
		StartTime time.Time
		*slog.Logger
		context.Context
		context.CancelCauseFunc
		handler                                                            ITask
		retry                                                              RetryConfig
		afterStartListeners, beforeDisposeListeners, afterDisposeListeners []func()
		Description
		startup, shutdown *util.Promise
		parent            *Job
		parentCtx         context.Context
		needRetry         bool
		state             TaskState
		level             byte
	}
)

func (*Task) keepalive() bool {
	return false
}

func (task *Task) GetState() TaskState {
	return task.state
}
func (task *Task) GetLevel() byte {
	return task.level
}
func (task *Task) GetParent() ITask {
	if task.parent != nil {
		return task.parent.handler
	}
	return nil
}
func (task *Task) SetRetry(maxRetry int, retryInterval time.Duration) {
	task.retry.MaxRetry = maxRetry
	task.retry.RetryInterval = retryInterval
}
func (task *Task) GetTaskID() uint32 {
	return task.ID
}
func (task *Task) GetOwnerType() string {
	if task.Description != nil {
		if ownerType, ok := task.Description["ownerType"]; ok {
			return ownerType.(string)
		}
	}
	return strings.TrimSuffix(reflect.TypeOf(task.handler).Elem().Name(), "Task")
}

func (*Task) GetTaskType() TaskType {
	return TASK_TYPE_TASK
}

func (task *Task) GetTask() *Task {
	return task
}

func (task *Task) getParent() *Job {
	return task.parent
}

func (task *Task) GetKey() uint32 {
	return task.ID
}

func (task *Task) WaitStarted() error {
	return task.startup.Await()
}

func (task *Task) WaitStopped() (err error) {
	err = task.startup.Await()
	if err != nil {
		return err
	}
	//if task.shutdown == nil {
	//	return task.StopReason()
	//}
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
		task.Error("task stop with nil error", "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType(), "parent", task.GetParent().GetOwnerType())
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

func (task *Task) OnBeforeDispose(listener func()) {
	task.beforeDisposeListeners = append(task.beforeDisposeListeners, listener)
}

func (task *Task) OnDispose(listener func()) {
	task.afterDisposeListeners = append(task.afterDisposeListeners, listener)
}

func (task *Task) GetSignal() any {
	return task.Done()
}

func (task *Task) checkRetry(err error) (bool, error) {
	if errors.Is(err, ErrTaskComplete) {
		return false, err
	}
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
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
			if task.Logger != nil {
				task.Error("panic", "error", err, "stack", string(debug.Stack()))
			}
		}
	}()
	task.StartTime = time.Now()
	if task.Logger != nil {
		task.Debug("task start", "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType())
	}
	task.state = TASK_STATE_STARTING
	if v, ok := task.handler.(TaskStarter); ok {
		err = v.Start()
	}
	if err == nil {
		task.state = TASK_STATE_STARTED
		task.ResetRetryCount()
		if runHandler, ok := task.handler.(TaskBlock); ok {
			task.state = TASK_STATE_RUNNING
			err = runHandler.Run()
			if err == nil {
				return ErrTaskComplete
			} else {
				task.needRetry, err = task.checkRetry(err)
			}
		}
	} else {
		task.needRetry, err = task.checkRetry(err)
		if task.needRetry {
			defer task.reStart()
		}
	}
	task.startup.Fulfill(err)
	if err != nil {
		return
	}
	for _, listener := range task.afterStartListeners {
		listener()
	}
	if goHandler, ok := task.handler.(TaskGo); ok {
		task.state = TASK_STATE_GOING
		go task.run(goHandler.Go)
	}
	return
}

func (task *Task) reStart() {
	if task.IsStopped() {
		task.Context, task.CancelCauseFunc = context.WithCancelCause(task.parentCtx)
		task.shutdown = util.NewPromise(context.Background())
	}
	task.startup = util.NewPromise(task.Context)
	parent := task.parent
	task.parent = nil
	parent.AddTask(task.handler)
}

func (task *Task) dispose() {
	if task.state < TASK_STATE_STARTED {
		return
	}
	reason := task.StopReason()
	task.state = TASK_STATE_DISPOSING
	if task.Logger != nil {
		taskType, ownerType := task.GetTaskType(), task.GetOwnerType()
		yargs := []any{"reason", reason, "taskId", task.ID, "taskType", taskType, "ownerType", ownerType}
		task.Debug("task dispose", yargs...)
		defer task.Debug("task disposed", yargs...)
	}
	for _, listener := range task.beforeDisposeListeners {
		listener()
	}
	if v, ok := task.handler.(TaskDisposal); ok {
		v.Dispose()
	}
	task.shutdown.Fulfill(reason)
	for _, listener := range task.afterDisposeListeners {
		listener()
	}
	task.state = TASK_STATE_DISPOSED
	if !errors.Is(reason, ErrTaskComplete) && task.needRetry {
		task.reStart()
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
