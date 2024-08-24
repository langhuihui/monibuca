package task

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"
)

func createMarcoTask() *MarcoTask {
	var mt MarcoTask
	mt.initTask(context.Background(), &mt)
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
	Task
}

func (task *retryDemoTask) Start() error {
	return io.ErrClosedPipe
}

func Test_RetryTask(t *testing.T) {
	mt := createMarcoTask()
	var demoTask retryDemoTask
	demoTask.SetRetry(3, time.Second)
	reason := mt.AddTask(&demoTask).WaitStopped()
	if !errors.Is(reason, ErrRetryRunOut) {
		t.Errorf("expected retry run out, got %v", reason)
	}
	if demoTask.retry.RetryCount != 3 {
		t.Errorf("expected 3 retries, got %d", demoTask.retry.RetryCount)
	}
}

func Test_Call_ExecutesCallback(t *testing.T) {
	mt := createMarcoTask()
	called := false
	mt.Call(func() error {
		called = true
		return nil
	})
	if !called {
		t.Errorf("expected callback to be called")
	}
}

func Test_StopByContext(t *testing.T) {
	mt := createMarcoTask()
	var task Task
	ctx, cancel := context.WithCancel(context.Background())
	mt.AddTaskWithContext(ctx, &task)
	time.AfterFunc(time.Millisecond*100, cancel)
	mt.WaitStopped()
	if !task.StopReasonIs(context.Canceled) {
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
	if !task.StopReasonIs(ErrAutoStop) {
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
