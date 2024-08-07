package pkg

import (
	"context"
	"io"
	"log/slog"
	"m7s.live/m7s/v5/pkg/util"
	"reflect"
	"slices"
	"sync/atomic"
	"time"
)

const TraceLevel = slog.Level(-8)

type TaskExecutor interface {
	Start() error
	Dispose()
}

type Task struct {
	ID        uint32
	StartTime time.Time
	*slog.Logger
	context.Context
	context.CancelCauseFunc
	Executor TaskExecutor
	started  *util.Promise
}

func (task *Task) GetTask() *Task {
	return task
}

func (task *Task) GetKey() uint32 {
	return task.ID
}

func (task *Task) Begin() (err error) {
	task.StartTime = time.Now()
	err = task.Executor.Start()
	task.started.Fulfill(err)
	return
}

func (task *Task) WaitStarted() error {
	return task.started.Await()
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

func (task *Task) Init(ctx context.Context, logger *slog.Logger) {
	task.Logger = logger
	task.Context, task.CancelCauseFunc = context.WithCancelCause(ctx)
	task.started = util.NewPromise(task.Context)
}

type CallBackTaskExecutor func()

func (call CallBackTaskExecutor) Start() error {
	call()
	return io.EOF
}

func (call CallBackTaskExecutor) Dispose() {
	// nothing to do, never called
}

type TaskManager struct {
	shutdown   *util.Promise
	stopReason error
	start      chan *Task
	Tasks      []*Task
	idG        atomic.Uint32
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		shutdown: util.NewPromise(context.TODO()),
		start:    make(chan *Task, 10),
	}
}

func (t *TaskManager) Add(task *Task) {
	t.start <- task
}

func (t *TaskManager) Call(callback CallBackTaskExecutor) {
	var tmpTask Task
	tmpTask.Init(context.TODO(), nil)
	tmpTask.Executor = callback
	_ = t.Start(&tmpTask)
}

func (t *TaskManager) Start(task *Task) error {
	t.start <- task
	return task.WaitStarted()
}

func (t *TaskManager) GetID() uint32 {
	return t.idG.Add(1)
}

// Run task Start and Dispose in this goroutine
func (t *TaskManager) Run(extra ...any) {
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(t.start)}}
	extraLen := len(extra) / 2
	var callbacks []reflect.Value
	for i := range extraLen {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(extra[i*2])})
		callbacks = append(callbacks, reflect.ValueOf(extra[i*2+1]))
	}
	defer func() {
		cases = slices.Delete(cases, 0, 1+extraLen)
		for len(cases) > 0 {
			chosen, _, _ := reflect.Select(cases)
			task := t.Tasks[chosen]
			task.Executor.Dispose()
			t.Tasks = slices.Delete(t.Tasks, chosen, chosen+1)
			cases = slices.Delete(cases, chosen, chosen+1)
		}
		t.shutdown.Fulfill(t.stopReason)
	}()
	for {
		if chosen, rev, ok := reflect.Select(cases); chosen == 0 {
			if !ok {
				return
			}
			task := rev.Interface().(*Task)
			if err := task.Begin(); err == nil {
				t.Tasks = append(t.Tasks, task)
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(task.Done())})
			} else {
				task.Stop(err)
			}
		} else if chosen <= extraLen {
			callbacks[chosen-1].Call([]reflect.Value{rev})
		} else {
			taskIndex := chosen - 1 - extraLen
			task := t.Tasks[taskIndex]
			task.Executor.Dispose()
			t.Tasks = slices.Delete(t.Tasks, taskIndex, taskIndex+1)
			cases = slices.Delete(cases, chosen, chosen+1)
		}
	}
}

// Run task Start and Dispose in another goroutine
//func (t *TaskManager) Run() {
//	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(t.Start)}}
//	defer func() {
//		cases = slices.Delete(cases, 0, 1)
//		for len(cases) > 0 {
//			chosen, _, _ := reflect.Select(cases)
//			t.Done <- t.Tasks[chosen]
//			t.Tasks = slices.Delete(t.Tasks, chosen, chosen+1)
//			cases = slices.Delete(cases, chosen, chosen+1)
//		}
//		close(t.Done)
//	}()
//	for {
//		if chosen, rev, ok := reflect.Select(cases); chosen == 0 {
//			if !ok {
//				return
//			}
//			task := rev.Interface().(*Task)
//			t.Tasks = append(t.Tasks, task)
//			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(task.Done())})
//		} else {
//			t.Done <- t.Tasks[chosen-1]
//			t.Tasks = slices.Delete(t.Tasks, chosen-1, chosen)
//			cases = slices.Delete(cases, chosen, chosen+1)
//		}
//	}
//}

// ShutDown wait all task dispose
func (t *TaskManager) ShutDown(err error) {
	t.Stop(err)
	_ = t.shutdown.Await()
}

func (t *TaskManager) Stop(err error) {
	t.stopReason = err
	close(t.start)
}
