package util

import (
	"context"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
)

var RootTask MarcoLongTask
var idG atomic.Uint32

func GetNextTaskID() uint32 {
	return idG.Add(1)
}

func init() {
	RootTask.initTask(context.Background(), &RootTask)
	RootTask.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
}

type MarcoLongTask struct {
	MarcoTask
}

func (m *MarcoLongTask) initTask(ctx context.Context, task ITask) {
	m.MarcoTask.initTask(ctx, task)
	m.keepAlive = true
}

func (m *MarcoLongTask) GetTaskType() string {
	return "long"
}

// MarcoTask include sub tasks
type MarcoTask struct {
	Task
	addSub    chan ITask
	children  []ITask
	lazyRun   sync.Once
	keepAlive bool
}

func (m *MarcoTask) GetTaskType() string {
	return "marco"
}

func (mt *MarcoTask) getMaroTask() *MarcoTask {
	return mt
}

func (mt *MarcoTask) initTask(ctx context.Context, task ITask) {
	mt.Task.initTask(ctx, task)
	mt.shutdown = nil
	mt.addSub = make(chan ITask, 10)
}

func (mt *MarcoTask) dispose() {
	reason := mt.StopReason()
	if mt.Logger != nil {
		mt.Debug("task dispose", "reason", reason, "taskId", mt.ID, "taskType", mt.GetTaskType(), "ownerType", mt.GetOwnerType())
	}
	mt.disposeHandler()
	close(mt.addSub)
	_ = mt.WaitStopped()
	if mt.Logger != nil {
		mt.Debug("task disposed", "reason", reason, "taskId", mt.ID, "taskType", mt.GetTaskType(), "ownerType", mt.GetOwnerType())
	}
	for _, listener := range mt.afterDisposeListeners {
		listener()
	}
}

func (mt *MarcoTask) lazyStart(t ITask) {
	task := t.getTask()
	if mt.IsStopped() {
		task.startup.Reject(mt.StopReason())
		return
	}
	if task.ID == 0 {
		task.ID = GetNextTaskID()
	}
	if task.parent == nil {
		task.parent = mt
	}
	if task.Logger == nil {
		task.Logger = mt.Logger
	}
	if task.startHandler == nil {
		task.startHandler = EmptyStart
	}
	if task.disposeHandler == nil {
		task.disposeHandler = EmptyDispose
	}
	mt.lazyRun.Do(func() {
		mt.shutdown = NewPromise(context.Background())
		go mt.run()
	})
	mt.addSub <- t
}

func (mt *MarcoTask) Range(callback func(task *Task, m *MarcoTask) bool) {
	for _, task := range mt.children {
		var m *MarcoTask
		if v, ok := task.(interface{ getMaroTask() *MarcoTask }); ok {
			m = v.getMaroTask()
		}
		callback(task.getTask(), m)
	}
}

func (mt *MarcoTask) AddTask(task ITask) *Task {
	t := task.getTask()
	if t.parentCtx != nil && task.IsStopped() { //reuse task
		t.parent = nil
		return mt.AddTaskWithContext(t.parentCtx, task)
	}
	return mt.AddTaskWithContext(mt.Context, task)
}

func (mt *MarcoTask) AddTaskWithContext(ctx context.Context, t ITask) (task *Task) {
	if ctx == nil && mt.Context == nil {
		panic("context is nil")
	}
	if task = t.getTask(); task.parent == nil {
		t.initTask(ctx, t)
	}
	mt.lazyStart(t)
	return
}

func (mt *MarcoTask) Call(callback func() error) {
	task := CreateTaskByCallBack(callback, nil)
	_ = mt.AddTask(task).WaitStarted()
}

func CreateTaskByCallBack(start func() error, dispose func()) *Task {
	var task Task
	task.startHandler = func() error {
		err := start()
		if err == nil && dispose == nil {
			err = ErrCallbackTask
		}
		return err
	}
	task.disposeHandler = dispose
	return &task
}

func (mt *MarcoTask) AddChan(channel any, callback any) *ChannelTask {
	var chanTask ChannelTask
	chanTask.initTask(mt.Context, &chanTask)
	chanTask.channel = reflect.ValueOf(channel)
	chanTask.callback = reflect.ValueOf(callback)
	mt.lazyStart(&chanTask)
	return &chanTask
}

func (mt *MarcoTask) run() {
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mt.addSub)}}
	defer func() {
		stopReason := mt.StopReason()
		for _, task := range mt.children {
			task.Stop(stopReason)
			if task.getParent() == mt {
				task.dispose()
			}
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
			if task := rev.Interface().(ITask); task.getParent() == mt {
				if err := task.start(); err == nil {
					mt.children = append(mt.children, task)
					cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: task.getSignal()})
				} else {
					task.Stop(err)
				}
			}
		} else {
			taskIndex := chosen - 1
			task := mt.children[taskIndex]
			if !ok {
				if task.getParent() == mt {
					task.dispose()
				}
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
