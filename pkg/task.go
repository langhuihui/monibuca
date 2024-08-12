package pkg

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
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
		getTask() *Task
		Stop(error)
		StopReason() error
		start(*MarcoTask) (reflect.Value, error)
		dispose(*MarcoTask)
		IsStopped() bool
	}
	IChannelTask interface {
		tick(reflect.Value)
	}
	iMacroMask interface {
		ITask
		macroTask() *MarcoTask
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
		startHandler      func() error
		disposeHandler    func()
		Description       map[string]any
		startup, shutdown *util.Promise
		parent            *MarcoTask
	}
	ChannelTask struct {
		stopReason error
		channel    reflect.Value
		callback   reflect.Value
		stop       func(error)
	}
	RetryTask struct {
		Task
		MaxRetry      int
		RetryCount    int
		RetryInterval time.Duration
	}
	IRetryTask interface {
		ITask
		getRetryTask() *RetryTask
	}
)

func (t *ChannelTask) getTask() *Task {
	return nil
}

func (t *ChannelTask) start(*MarcoTask) (reflect.Value, error) {
	return t.channel, nil
}

func (t *ChannelTask) dispose(*MarcoTask) {

}

func (t *ChannelTask) Stop(err error) {
	t.stopReason = err
	if t.stop != nil {
		t.stop(err)
	}
}

func (t *ChannelTask) IsStopped() bool {
	return t.stopReason != nil
}

func (t *ChannelTask) StopReason() error {
	return t.stopReason
}

func (t *ChannelTask) tick(signal reflect.Value) {
	t.callback.Call([]reflect.Value{signal})
}

func (task *Task) getTask() *Task {
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
		if task.Logger != nil {
			task.Debug("task stop", "reason", err.Error(), "elapsed", time.Since(task.StartTime), "taskId", task.ID)
		}
		task.CancelCauseFunc(err)
	}
}

func (task *Task) start(mt *MarcoTask) (signal reflect.Value, err error) {
	if task.parent != mt {
		return
	}
	task.StartTime = time.Now()
	err = task.startHandler()
	if task.Logger != nil {
		task.Debug("task start", "taskId", task.ID)
	}
	task.startup.Fulfill(err)
	signal = reflect.ValueOf(task.Done())
	return
}

func (task *Task) dispose(mt *MarcoTask) {
	if task.parent != mt {
		return
	}
	reason := task.StopReason()
	if task.Logger != nil {
		task.Debug("task dispose", "reason", reason, "taskId", task.ID)
	}
	task.disposeHandler()
	task.shutdown.Fulfill(reason)
}

func (task *Task) init(ctx context.Context) {
	task.Context, task.CancelCauseFunc = context.WithCancelCause(ctx)
	task.startup = util.NewPromise(task.Context)
	task.shutdown = util.NewPromise(task.Context)
}

type CallBack func(*Task) error

type MarcoLongTask struct {
	MarcoTask
}

func (task *MarcoLongTask) start(mt *MarcoTask) (signal reflect.Value, err error) {
	task.keepAlive = true
	return task.Task.start(mt)
}

func (r *RetryTask) getRetryTask() *RetryTask {
	return r
}

// MarcoTask include sub tasks
type MarcoTask struct {
	Task
	addSub    chan ITask
	children  []ITask
	idG       atomic.Uint32
	lazyRun   sync.Once
	keepAlive bool
}

func (mt *MarcoTask) macroTask() *MarcoTask {
	return mt
}

func (mt *MarcoTask) lazyStart(t ITask) {
	mt.lazyRun.Do(func() {
		mt.addSub = make(chan ITask, 10)
		go mt.run()
	})
	mt.addSub <- t
}

func (mt *MarcoTask) AddTask(task ITask) *Task {
	return mt.AddTaskWithContext(mt.Context, task)
}

func (mt *MarcoTask) AddTaskWithContext(ctx context.Context, t ITask) (task *Task) {
	task = t.getTask()
	task.init(ctx)
	if mt.IsStopped() {
		task.startup.Reject(mt.StopReason())
		return
	}
	if task.ID == 0 {
		task.ID = mt.GetID()
	}
	if task.parent == nil {
		s, d := EmptyStart, EmptyDispose
		if v, ok := t.(TaskStarter); ok {
			s = v.Start
		}
		if v, ok := t.(TaskDisposal); ok {
			d = v.Dispose
		}
		task.parent = mt
		if v, ok := t.(iMacroMask); ok {
			m := v.macroTask()
			task.disposeHandler = func() {
				close(m.addSub)
				_ = m.shutdown.Await()
				d()
			}
		} else {
			task.disposeHandler = d
		}
		task.startHandler = s
	}
	mt.lazyStart(t)
	return
}

func (mt *MarcoTask) Call(callback CallBack) {
	task := mt.AddCall(callback, nil)
	_ = task.WaitStarted()
}

func (mt *MarcoTask) AddCall(start CallBack, dispose func()) *Task {
	var task Task
	task.init(mt.Context)
	if mt.IsStopped() {
		task.startup.Reject(mt.StopReason())
		return &task
	}
	if task.ID == 0 {
		task.ID = mt.GetID()
	}
	task.parent = mt
	task.startHandler = func() error {
		err := start(&task)
		if err == nil && dispose == nil {
			err = ErrCallbackTask
		}
		return err
	}
	if dispose == nil {
		task.disposeHandler = EmptyDispose
	} else {
		task.disposeHandler = dispose
	}
	mt.lazyStart(&task)
	return &task
}

func (mt *MarcoTask) AddChan(channel any, callback any, stop func(error)) {
	var chanTask ChannelTask
	chanTask.channel = reflect.ValueOf(channel)
	chanTask.callback = reflect.ValueOf(callback)
	chanTask.stop = stop
	mt.lazyStart(&chanTask)
}

func (mt *MarcoTask) GetID() uint32 {
	return mt.idG.Add(1)
}

func (mt *MarcoTask) run() {
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mt.addSub)}}
	defer func() {
		stopReason := mt.StopReason()
		for _, task := range mt.children {
			task.Stop(stopReason)
			task.dispose(mt)
		}
		mt.children = nil
		mt.addSub = nil
		mt.shutdown.Fulfill(stopReason)
	}()
	for {
		if chosen, rev, ok := reflect.Select(cases); chosen == 0 {
			if !ok {
				return
			}
			task := rev.Interface().(ITask)
			for !mt.IsStopped() && !task.IsStopped() {
				if signal, err := task.start(mt); err == nil {
					mt.children = append(mt.children, task)
					cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: signal})
					break
				} else if r, ok := task.(IRetryTask); ok {
					if t := r.getRetryTask(); t.MaxRetry < 0 || t.RetryCount < t.MaxRetry {
						t.RetryCount++
						if delta := time.Since(t.StartTime); delta < t.RetryInterval {
							time.Sleep(t.RetryInterval - delta)
						}
						if t.Logger != nil {
							t.Warn("retry", "count", t.RetryCount, "total", t.MaxRetry)
						}
					} else {
						task.Stop(ErrRetryRunOut)
						break
					}
				} else {
					task.Stop(err)
					break
				}
			}
		} else {
			taskIndex := chosen - 1
			task := mt.children[taskIndex]
			if !ok {
				task.dispose(mt)
				mt.children = slices.Delete(mt.children, taskIndex, taskIndex+1)
				cases = slices.Delete(cases, chosen, chosen+1)

			} else if c, ok := task.(IChannelTask); ok {
				c.tick(rev)
			}
		}
		if !mt.keepAlive && len(mt.children) == 0 {
			mt.Stop(ErrAutoStop)
		}
	}
}
