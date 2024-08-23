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
		"ownerType": "root",
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

func (*MarcoLongTask) GetTaskType() TaskType {
	return TASK_TYPE_LONG_MACRO
}

// MarcoTask include sub tasks
type MarcoTask struct {
	Task
	addSub                chan ITask
	children              []ITask
	lazyRun               sync.Once
	keepAlive             bool
	childrenDisposed      chan struct{}
	childDisposeListeners []func(ITask)
	blocked               bool
}

func (*MarcoTask) GetTaskType() TaskType {
	return TASK_TYPE_MACRO
}

func (mt *MarcoTask) Blocked() bool {
	return mt.blocked
}

func (mt *MarcoTask) waitChildrenDispose() {
	close(mt.addSub)
	<-mt.childrenDisposed
}

func (mt *MarcoTask) OnChildDispose(listener func(ITask)) {
	mt.childDisposeListeners = append(mt.childDisposeListeners, listener)
}

func (mt *MarcoTask) onChildDispose(child ITask) {
	for _, listener := range mt.childDisposeListeners {
		listener(child)
	}
	if mt.parent != nil {
		mt.parent.onChildDispose(child)
	}
	if child.getParent() == mt {
		child.dispose()
	}
}

func (mt *MarcoTask) dispose() {
	if mt.childrenDisposed != nil {
		mt.OnBeforeDispose(mt.waitChildrenDispose)
	}
	mt.Task.dispose()
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
		task.level = mt.level + 1
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
		mt.childrenDisposed = make(chan struct{})
		mt.addSub = make(chan ITask, 10)
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

func (mt *MarcoTask) addChild(task ITask) int {
	mt.children = append(mt.children, task)
	return len(mt.children) - 1
}

func (mt *MarcoTask) removeChild(index int) {
	mt.onChildDispose(mt.children[index])
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
			mt.onChildDispose(task)
		}
		mt.children = nil
		close(mt.childrenDisposed)
	}()
	for {
		mt.blocked = false
		if chosen, rev, ok := reflect.Select(cases); chosen == 0 {
			mt.blocked = true
			if !ok {
				return
			}
			if task := rev.Interface().(ITask); task.getParent() == mt {
				index := mt.addChild(task)
				if err := task.start(); err == nil {
					cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(task.GetSignal())})
				} else {
					task.Stop(err)
					mt.removeChild(index)
				}
			} else {
				mt.addChild(task)
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(task.GetSignal())})
			}
		} else {
			taskIndex := chosen - 1
			task := mt.children[taskIndex]
			switch tt := task.(type) {
			case IChannelTask:
				tt.Tick(rev.Interface())
			}
			if !ok {
				mt.removeChild(taskIndex)
				cases = slices.Delete(cases, chosen, chosen+1)
			}
		}
		if !mt.keepAlive && len(mt.children) == 0 {
			mt.Stop(ErrAutoStop)
		}
	}
}
