package pkg

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"sync/atomic"
	"time"

	"m7s.live/m7s/v5/pkg/util"
)

const TraceLevel = slog.Level(-8)

var (
	ErrAutoStop     = errors.New("auto stop")
	ErrCallbackTask = errors.New("callback task")
)

type getTask interface{ GetTask() *Task }
type TaskExecutor interface {
	Start() error
	Dispose()
}

type TempTaskExecutor struct {
	StartFunc   func() error
	DisposeFunc func()
}

func (t TempTaskExecutor) Start() error {
	if t.StartFunc == nil {
		return nil
	}
	return t.StartFunc()
}

func (t TempTaskExecutor) Dispose() {
	if t.DisposeFunc != nil {
		t.DisposeFunc()
	}
}

type Task struct {
	ID        uint32
	StartTime time.Time
	*slog.Logger
	context.Context
	context.CancelCauseFunc
	exe               TaskExecutor
	Description       map[string]any
	startup, shutdown *util.Promise
	parent            *MarcoTask
}

func (task *Task) GetTask() *Task {
	return task
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
		task.Info("stop", "reason", err.Error())
		task.CancelCauseFunc(err)
	}
}

func (task *Task) Init(ctx context.Context, logger *slog.Logger, executor TaskExecutor) {
	task.Logger = logger
	task.exe = executor
	task.Context, task.CancelCauseFunc = context.WithCancelCause(ctx)
	task.startup = util.NewPromise(task.Context)
	task.shutdown = util.NewPromise(task.Context)
}

type CallBack func(*Task) error

// MarcoTask include sub tasks
type MarcoTask struct {
	Task
	KeepAlive      bool
	exe            TaskExecutor
	addSub         chan *Task
	subTasks       []*Task
	extraCases     []reflect.SelectCase
	extraCallbacks []reflect.Value
	idG            atomic.Uint32
}

func (mt *MarcoTask) Init(ctx context.Context, logger *slog.Logger, executor TaskExecutor, extra ...any) {
	mt.Task.Init(ctx, logger, mt)
	mt.exe = executor
	for i := range len(extra) / 2 {
		mt.extraCases = append(mt.extraCases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(extra[i*2])})
		mt.extraCallbacks = append(mt.extraCallbacks, reflect.ValueOf(extra[i*2+1]))
	}
}

func (mt *MarcoTask) InitKeepAlive(ctx context.Context, logger *slog.Logger, executor TaskExecutor, extra ...any) {
	mt.Init(ctx, logger, executor, extra...)
	mt.KeepAlive = true
}

func (mt *MarcoTask) Start() error {
	if mt.exe != nil {
		return mt.exe.Start()
	}
	return nil
}

func (mt *MarcoTask) AddTasks(getTasks ...getTask) {
	for _, getTask := range getTasks {
		mt.AddTask(getTask)
	}
}

func (mt *MarcoTask) AddTask(getTask getTask) {
	if mt.IsStopped() {
		getTask.GetTask().startup.Reject(mt.StopReason())
		return
	}
	if mt.addSub == nil {
		mt.addSub = make(chan *Task, 10)
		go mt.run()
	}
	task := getTask.GetTask()
	if task.ID == 0 {
		task.ID = mt.GetID()
	}
	if task.parent == nil {
		task.parent = mt
		if v, ok := getTask.(TaskExecutor); ok {
			task.exe = v
		}
	}
	mt.addSub <- task
}

func (mt *MarcoTask) Call(callback CallBack) {
	task := mt.AddCall(callback, nil)
	_ = task.WaitStarted()
}

func (mt *MarcoTask) AddCall(start CallBack, dispose func(*Task)) *Task {
	var tmpTask Task
	var tmpExe TempTaskExecutor
	if start != nil {
		tmpExe.StartFunc = func() error {
			err := start(&tmpTask)
			if err == nil && dispose == nil {
				err = ErrCallbackTask
			}
			return err
		}
	}
	if dispose != nil {
		tmpExe.DisposeFunc = func() {
			dispose(&tmpTask)
		}
	}
	tmpTask.Init(mt.Context, nil, tmpExe)
	mt.AddTask(&tmpTask)
	return &tmpTask
}

func (mt *MarcoTask) WaitTaskAdded(getTask getTask) error {
	mt.AddTask(getTask)
	return getTask.GetTask().WaitStarted()
}

func (mt *MarcoTask) GetID() uint32 {
	return mt.idG.Add(1)
}

func (mt *MarcoTask) startSubTask(task *Task) (err error) {
	if task.startup.IsPending() {
		task.StartTime = time.Now()
		err = task.exe.Start()
		if task.Logger != nil {
			task.Debug("start")
		}
		task.startup.Fulfill(err)
	}
	return
}

func (mt *MarcoTask) disposeSubTask(task *Task, reason error) {
	if task.parent != mt {
		return
	}
	if task.Logger != nil {
		task.Debug("dispose", "reason", reason)
	}
	task.exe.Dispose()
	if m, ok := task.exe.(*MarcoTask); ok {
		m.WaitStopped()
	} else {
		task.shutdown.Fulfill(reason)
	}
}

func (mt *MarcoTask) run() {
	extraLen := len(mt.extraCases)
	cases := append([]reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mt.addSub)}}, mt.extraCases...)
	defer func() {
		stopReason := mt.StopReason()
		for _, task := range mt.subTasks {
			task.Stop(stopReason)
			mt.disposeSubTask(task, stopReason)
		}
		mt.subTasks = nil
		mt.addSub = nil
		mt.extraCases = nil
		mt.extraCallbacks = nil
		mt.shutdown.Fulfill(stopReason)
	}()
	for {
		if chosen, rev, ok := reflect.Select(cases); chosen == 0 {
			if !ok {
				return
			}
			task := rev.Interface().(*Task)
			if err := mt.startSubTask(task); err == nil {
				mt.subTasks = append(mt.subTasks, task)
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(task.Done())})
			} else {
				if task.Logger != nil {
					task.Warn("start failed", "error", err)
				}
				task.Stop(err)
			}
		} else if chosen <= extraLen {
			mt.extraCallbacks[chosen-1].Call([]reflect.Value{rev})
		} else {
			taskIndex := chosen - extraLen - 1
			task := mt.subTasks[taskIndex]
			mt.disposeSubTask(task, task.StopReason())
			mt.subTasks = slices.Delete(mt.subTasks, taskIndex, taskIndex+1)
			cases = slices.Delete(cases, chosen, chosen+1)
			if !mt.KeepAlive && len(mt.subTasks) == 0 {
				mt.Stop(ErrAutoStop)
			}
		}
	}
}

// ShutDown wait all task dispose
func (mt *MarcoTask) ShutDown(err error) {
	mt.Stop(err)
	_ = mt.shutdown.Await()
}

func (mt *MarcoTask) Dispose() {
	if mt.exe != nil {
		mt.exe.Dispose()
	}
	close(mt.addSub)
}
