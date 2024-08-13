package pkg

import (
	"context"
	"log/slog"
	"m7s.live/m7s/v5/pkg/util"
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
	RootTask.init(context.Background())
	RootTask.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	RootTask.Name = "Root"
}

type MarcoLongTask struct {
	MarcoTask
}

func (task *MarcoLongTask) start() (signal reflect.Value, err error) {
	task.keepAlive = true
	return task.MarcoTask.start()
}

// MarcoTask include sub tasks
type MarcoTask struct {
	Task
	addSub    chan ITask
	children  []ITask
	lazyRun   sync.Once
	keepAlive bool
}

func (mt *MarcoTask) init(ctx context.Context) {
	mt.Task.init(ctx)
	mt.shutdown = nil
	mt.addSub = make(chan ITask, 10)
}

func (mt *MarcoTask) dispose() {
	reason := mt.StopReason()
	if mt.Logger != nil {
		mt.Debug("task dispose", "reason", reason, "taskId", mt.ID, "taskName", mt.Name)
	}
	mt.disposeHandler()
	close(mt.addSub)
	_ = mt.WaitStopped()
	if mt.Logger != nil {
		mt.Debug("task disposed", "reason", reason, "taskId", mt.ID, "taskName", mt.Name)
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
		mt.shutdown = util.NewPromise(context.Background())
		go mt.run()
	})
	mt.addSub <- t
}

func (mt *MarcoTask) AddTask(task ITask) *Task {
	return mt.AddTaskWithContext(mt.Context, task)
}

func (mt *MarcoTask) AddTaskWithContext(ctx context.Context, t ITask) (task *Task) {
	if task = t.getTask(); task.parent == nil {
		t.init(ctx)
		if v, ok := t.(TaskStarter); ok {
			task.startHandler = v.Start
		}
		if v, ok := t.(TaskDisposal); ok {
			task.disposeHandler = v.Dispose
		}
	}
	mt.lazyStart(t)
	return
}

type CallBack func(*Task) error

func (mt *MarcoTask) Call(callback CallBack) {
	task := mt.AddCall(callback, nil)
	_ = task.WaitStarted()
}

func (mt *MarcoTask) AddCall(start CallBack, dispose func()) *Task {
	var task Task
	task.init(mt.Context)
	task.startHandler = func() error {
		err := start(&task)
		if err == nil && dispose == nil {
			err = ErrCallbackTask
		}
		return err
	}
	task.disposeHandler = dispose
	mt.lazyStart(&task)
	return &task
}

func (mt *MarcoTask) AddChan(channel any, callback any) *ChannelTask {
	var chanTask ChannelTask
	chanTask.init(mt.Context)
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
				if signal, err := task.start(); err == nil {
					mt.children = append(mt.children, task)
					cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: signal})
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
