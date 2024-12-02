package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"m7s.live/v5/pkg/util"
)

const TraceLevel = slog.Level(-8)
const OwnerTypeKey = "ownerType"

var (
	ErrAutoStop     = errors.New("auto stop")
	ErrRetryRunOut  = errors.New("retry out")
	ErrStopByUser   = errors.New("stop by user")
	ErrRestart      = errors.New("restart")
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
		start() bool
		dispose()
		checkRetry(error) bool
		reset()
		IsStopped() bool
		GetTaskType() TaskType
		GetOwnerType() string
		GetDescriptions() map[string]string
		SetDescription(key string, value any)
		SetDescriptions(value Description)
		SetRetry(maxRetry int, retryInterval time.Duration)
		Depend(ITask)
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
		Blocked() ITask
		Call(func() error, ...any)
		Post(func() error, ...any) *Task
	}
	IChannelTask interface {
		ITask
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
	Description    = map[string]any
	TaskContextKey string
	Task           struct {
		ID          uint32
		StartTime   time.Time
		StartReason string
		*slog.Logger
		context.Context
		context.CancelCauseFunc
		handler                                                            ITask
		retry                                                              RetryConfig
		afterStartListeners, beforeDisposeListeners, afterDisposeListeners []func()
		description                                                        sync.Map
		startup, shutdown                                                  *util.Promise
		parent                                                             *Job
		parentCtx                                                          context.Context
		state                                                              TaskState
		level                                                              byte
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
	if ownerType, ok := task.description.Load(OwnerTypeKey); ok {
		return ownerType.(string)
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

func (task *Task) StopReasonIs(errs ...error) bool {
	stopReason := task.StopReason()
	for _, err := range errs {
		if errors.Is(err, stopReason) {
			return true
		}
	}
	return false
}

func (task *Task) Stop(err error) {
	if err == nil {
		task.Error("task stop with nil error", "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType(), "parent", task.GetParent().GetOwnerType())
		panic("task stop with nil error")
	}
	if task.CancelCauseFunc != nil {
		if tt := task.handler.GetTaskType(); task.Logger != nil && tt != TASK_TYPE_CALL {
			task.Debug("task stop", "reason", err, "elapsed", time.Since(task.StartTime), "taskId", task.ID, "taskType", tt, "ownerType", task.GetOwnerType())
		}
		task.CancelCauseFunc(err)
	}
}

func (task *Task) Depend(t ITask) {
	t.OnDispose(func() {
		task.Stop(t.StopReason())
	})
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

func (task *Task) checkRetry(err error) bool {
	if errors.Is(err, ErrTaskComplete) || errors.Is(err, ErrExit) || errors.Is(err, ErrStopByUser) {
		return false
	}
	if task.parent.IsStopped() {
		return false
	}
	if task.retry.MaxRetry < 0 || task.retry.RetryCount < task.retry.MaxRetry {
		task.retry.RetryCount++
		task.SetDescription("retryCount", task.retry.RetryCount)
		if task.Logger != nil {
			if task.retry.MaxRetry < 0 {
				task.Warn(fmt.Sprintf("retry %d/∞", task.retry.RetryCount), "taskId", task.ID)
			} else {
				task.Warn(fmt.Sprintf("retry %d/%d", task.retry.RetryCount, task.retry.MaxRetry), "taskId", task.ID)
			}
		}
		if delta := time.Since(task.StartTime); delta < task.retry.RetryInterval {
			time.Sleep(task.retry.RetryInterval - delta)
		}
		return true
	} else {
		if task.retry.MaxRetry > 0 {
			if task.Logger != nil {
				task.Warn(fmt.Sprintf("max retry %d failed", task.retry.MaxRetry))
			}
			return false
		}
	}
	return errors.Is(err, ErrRestart)
}

func (task *Task) start() bool {
	var err error
	if !ThrowPanic {
		defer func() {
			if r := recover(); r != nil {
				err = errors.New(fmt.Sprint(r))
				if task.Logger != nil {
					task.Error("panic", "error", err, "stack", string(debug.Stack()))
				}
			}
		}()
	}
	for {
		task.StartTime = time.Now()
		if tt := task.handler.GetTaskType(); task.Logger != nil && tt != TASK_TYPE_CALL {
			task.Debug("task start", "taskId", task.ID, "taskType", tt, "ownerType", task.GetOwnerType(), "reason", task.StartReason)
		}
		task.state = TASK_STATE_STARTING
		if v, ok := task.handler.(TaskStarter); ok {
			err = v.Start()
		}
		if err == nil {
			task.state = TASK_STATE_STARTED
			task.startup.Fulfill(err)
			for _, listener := range task.afterStartListeners {
				if task.IsStopped() {
					break
				}
				listener()
			}
			if task.IsStopped() {
				err = task.StopReason()
			} else {
				task.ResetRetryCount()
				if runHandler, ok := task.handler.(TaskBlock); ok {
					task.state = TASK_STATE_RUNNING
					err = runHandler.Run()
					if err == nil {
						err = ErrTaskComplete
					}
				}
			}
		}
		if err == nil {
			if goHandler, ok := task.handler.(TaskGo); ok {
				task.state = TASK_STATE_GOING
				go task.run(goHandler.Go)
			}
			return true
		} else {
			task.Stop(err)
			task.parent.onChildDispose(task.handler)
			if task.checkRetry(err) {
				task.reset()
			} else {
				return false
			}
		}
	}
}

func (task *Task) reset() {
	task.Context, task.CancelCauseFunc = context.WithCancelCause(task.parentCtx)
	task.shutdown = util.NewPromise(context.Background())
	task.startup = util.NewPromise(task.Context)
}

func (task *Task) GetDescriptions() map[string]string {
	return maps.Collect(func(yield func(key, value string) bool) {
		task.description.Range(func(key, value any) bool {
			return yield(key.(string), fmt.Sprintf("%+v", value))
		})
	})
}

func (task *Task) SetDescription(key string, value any) {
	task.description.Store(key, value)
}

func (task *Task) RemoveDescription(key string) {
	task.description.Delete(key)
}

func (task *Task) SetDescriptions(value Description) {
	for k, v := range value {
		task.description.Store(k, v)
	}
}

func (task *Task) dispose() {
	if task.state < TASK_STATE_STARTED {
		return
	}
	reason := task.StopReason()
	task.state = TASK_STATE_DISPOSING
	if task.Logger != nil {
		taskType, ownerType := task.handler.GetTaskType(), task.GetOwnerType()
		if taskType != TASK_TYPE_CALL {
			yargs := []any{"reason", reason, "taskId", task.ID, "taskType", taskType, "ownerType", ownerType}
			task.Debug("task dispose", yargs...)
			defer task.Debug("task disposed", yargs...)
		}
	}
	befores := len(task.beforeDisposeListeners)
	for i, listener := range task.beforeDisposeListeners {
		task.SetDescription("disposeProcess", fmt.Sprintf("b:%d/%d", i, befores))
		listener()
	}
	if job, ok := task.handler.(IJob); ok {
		mt := job.getJob()
		task.SetDescription("disposeProcess", "wait children")
		mt.eventLoopLock.Lock()
		if mt.addSub != nil {
			mt.waitChildrenDispose()
			mt.lazyRun = sync.Once{}
		}
		mt.eventLoopLock.Unlock()
	}
	task.SetDescription("disposeProcess", "self")
	if v, ok := task.handler.(TaskDisposal); ok {
		v.Dispose()
	}
	task.shutdown.Fulfill(reason)
	afters := len(task.afterDisposeListeners)
	for i, listener := range task.afterDisposeListeners {
		task.SetDescription("disposeProcess", fmt.Sprintf("a:%d/%d", i, afters))
		listener()
	}
	task.SetDescription("disposeProcess", "done")
	task.state = TASK_STATE_DISPOSED
}

func (task *Task) ResetRetryCount() {
	task.retry.RetryCount = 0
}

func (task *Task) run(handler func() error) {
	var err error
	defer func() {
		if !ThrowPanic {
			if r := recover(); r != nil {
				err = errors.New(fmt.Sprint(r))
				if task.Logger != nil {
					task.Error("panic", "error", err, "stack", string(debug.Stack()))
				}
			}
		}
		if err == nil {
			task.Stop(ErrTaskComplete)
		} else {
			task.Stop(err)
		}
	}()
	err = handler()
}
