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

var idG atomic.Uint32

func GetNextTaskID() uint32 {
	return idG.Add(1)
}

var RootTask MarcoLongTask

func init() {
	RootTask.initTask(context.Background(), &RootTask)
	RootTask.Description = map[string]any{
		"title": "RootTask",
	}
	RootTask.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
}

func ShutdownRootTask() {
	RootTask.Stop(ErrExit)
	RootTask.dispose()
}

type MarcoLongTask struct {
	MarcoTask
}

func (m *MarcoLongTask) initTask(ctx context.Context, task ITask) {
	m.MarcoTask.initTask(ctx, task)
	m.keepAlive = true
}

func (*MarcoLongTask) GetTaskType() string {
	return "long"
}
func (*MarcoLongTask) GetTaskTypeID() byte {
	return 2
}

// MarcoTask include sub tasks
type MarcoTask struct {
	Task
	addSub       chan ITask
	children     []ITask
	lazyRun      sync.Once
	keepAlive    bool
	addListeners []func(task ITask)
}

func (*MarcoTask) GetTaskType() string {
	return "marco"
}

func (*MarcoTask) GetTaskTypeID() byte {
	return 1
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
	task := t.GetTask()
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

func (mt *MarcoTask) RangeSubTask(callback func(task ITask) bool) {
	for _, task := range mt.children {
		callback(task)
	}
}

func (mt *MarcoTask) AddTask(task ITask) *Task {
	return mt.AddTaskWithContext(mt.Context, task)
}

func (mt *MarcoTask) OnTaskAdded(f func(ITask)) {
	mt.addListeners = append(mt.addListeners, f)
}

func (mt *MarcoTask) AddTaskWithContext(ctx context.Context, t ITask) (task *Task) {
	if ctx == nil && mt.Context == nil {
		panic("context is nil")
	}
	if task = t.GetTask(); task.parent == nil {
		t.initTask(ctx, t)
	}
	mt.lazyStart(t)
	return
}

func (mt *MarcoTask) Call(callback func() error) {
	mt.Post(callback).WaitStarted()
}

func (mt *MarcoTask) Post(callback func() error) *Task {
	task := CreateTaskByCallBack(callback, nil)
	return mt.AddTask(task)
}

type CallBackTask struct {
	Task
}

func CreateTaskByCallBack(start func() error, dispose func()) ITask {
	var task CallBackTask
	task.startHandler = func() error {
		err := start()
		if err == nil && dispose == nil {
			err = ErrTaskComplete
		}
		return err
	}
	task.disposeHandler = dispose
	return &task
}

func (mt *MarcoTask) addChild(task ITask) int {
	mt.children = append(mt.children, task)
	for _, listener := range mt.addListeners {
		listener(task)
	}
	return len(mt.children) - 1
}

func (mt *MarcoTask) removeChild(index int) {
	mt.children = slices.Delete(mt.children, index, index+1)
}

func (mt *MarcoTask) run() {
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mt.addSub)}}
	defer func() {
		err := recover()
		if err != nil {
			mt.Stop(err.(error))
		}
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
				index := mt.addChild(task)
				if err := task.start(); err == nil {
					cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(task.GetSignal())})
				} else {
					mt.removeChild(index)
					task.Stop(err)
				}
			} else {
				mt.children = append(mt.children, task)
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(task.GetSignal())})
			}
		} else {
			taskIndex := chosen - 1
			task := mt.children[taskIndex]
			if !ok {
				if task.getParent() == mt {
					task.dispose()
				}
				mt.removeChild(taskIndex)
				cases = slices.Delete(cases, chosen, chosen+1)

			} else if c, ok := task.(IChannelTask); ok {
				c.Tick(rev.Interface())
			}
		}
		if !mt.keepAlive && len(mt.children) == 0 {
			mt.Stop(ErrAutoStop)
		}
	}
}
