package util

import (
	"context"
	"log/slog"
	"m7s.live/m7s/v5/pkg"
	"os"
	"testing"
	"time"
)

func createMarcoTask() *MarcoTask {
	var mt MarcoTask
	mt.init(context.Background())
	mt.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	return &mt
}

func init() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func Test_AddTask_AddsTaskSuccessfully(t *testing.T) {
	mt := createMarcoTask()
	task := &Task{}
	mt.AddTask(task).WaitStarted()
	if len(mt.children) != 1 {
		t.Errorf("expected 1 child task, got %d", len(mt.children))
	}
}

type retryDemoTask struct {
	RetryTask
}

func (task *retryDemoTask) Start() error {
	return pkg.ErrRestart
}

func Test_RetryTask(t *testing.T) {
	mt := createMarcoTask()
	var demoTask retryDemoTask
	demoTask.MaxRetry = 3
	demoTask.RetryInterval = time.Second
	reason := mt.AddTask(&demoTask).WaitStopped()
	if reason != ErrAutoStop {
		t.Errorf("expected retry run out, got %v", reason)
	}
	if demoTask.RetryCount != 3 {
		t.Errorf("expected 3 retries, got %d", demoTask.RetryCount)
	}
}

func Test_Call_ExecutesCallback(t *testing.T) {
	mt := createMarcoTask()
	called := false
	mt.Call(func(*Task) error {
		called = true
		return nil
	})
	if !called {
		t.Errorf("expected callback to be called")
	}
}

func Test_AddCall_AddsCallbackTask(t *testing.T) {
	mt := createMarcoTask()
	called := false
	task := mt.AddCall(func(*Task) error {
		return nil
	}, func() {
		called = true
	})
	task.Stop(ErrCallbackTask)
	mt.WaitStopped()
	if !called {
		t.Errorf("expected callback to be called")
	}
}

func Test_AddChan_AddsChannelTask(t *testing.T) {
	mt := createMarcoTask()
	channel := time.NewTimer(time.Millisecond * 100)
	called := false
	callback := func(time.Time) {
		called = true
	}
	mt.AddChan(channel.C, callback)
	time.AfterFunc(time.Millisecond*500, func() {
		if !called {
			t.Errorf("expected callback to be called")
		}
	})
}

func Test_StopByContext(t *testing.T) {
	mt := createMarcoTask()
	var task Task
	ctx, cancel := context.WithCancel(context.Background())
	mt.AddTaskWithContext(ctx, &task)
	time.AfterFunc(time.Millisecond*100, cancel)
	mt.WaitStopped()
	if task.StopReason() != context.Canceled {
		t.Errorf("expected task to be stopped by context")
	}
}

func Test_ParentStop(t *testing.T) {
	mt := createMarcoTask()
	parent := &MarcoTask{}
	mt.AddTask(parent)
	var task Task
	parent.AddTask(&task)
	parent.Stop(ErrAutoStop)
	parent.WaitStopped()
	if task.StopReason() != ErrAutoStop {
		t.Errorf("expected task to be stopped")
	}
}

func Test_Hooks(t *testing.T) {
	mt := createMarcoTask()
	called := 0
	var task Task
	task.OnStart(func() {
		called++
		if called != 1 {
			t.Errorf("expected 1, got %d", called)
		}
	})
	task.OnDispose(func() {
		called++
		if called != 3 {
			t.Errorf("expected 3, got %d", called)
		}
	})
	task.OnStart(func() {
		called++
		if called != 2 {
			t.Errorf("expected 2, got %d", called)
		}
	})
	task.OnDispose(func() {
		called++
		if called != 4 {
			t.Errorf("expected 4, got %d", called)
		}
	})
	mt.AddTask(&task).WaitStarted()
}
