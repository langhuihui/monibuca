package task

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

var root Work

func init() {
	root.Context, root.CancelCauseFunc = context.WithCancelCause(context.Background())
	root.handler = &root
	root.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func Test_AddTask_AddsTaskSuccessfully(t *testing.T) {
	var task Task
	root.AddTask(&task)
	_ = task.WaitStarted()
	if len(root.children) != 1 {
		t.Errorf("expected 1 child task, got %d", len(root.children))
	}
}

type retryDemoTask struct {
	Task
}

func (task *retryDemoTask) Start() error {
	return io.ErrClosedPipe
}

func Test_RetryTask(t *testing.T) {
	var demoTask retryDemoTask
	var parent Job
	root.AddTask(&parent)
	demoTask.SetRetry(3, time.Second)
	parent.AddTask(&demoTask)
	_ = parent.WaitStopped()
	if demoTask.retry.RetryCount != 3 {
		t.Errorf("expected 3 retries, got %d", demoTask.retry.RetryCount)
	}
}

func Test_Call_ExecutesCallback(t *testing.T) {
	called := false
	root.Call(func() error {
		called = true
		return nil
	})
	if !called {
		t.Errorf("expected callback to be called")
	}
}

func Test_StopByContext(t *testing.T) {
	var task Task
	ctx, cancel := context.WithCancel(context.Background())
	root.AddTask(&task, ctx)
	time.AfterFunc(time.Millisecond*100, cancel)
	if !errors.Is(task.WaitStopped(), context.Canceled) {
		t.Errorf("expected task to be stopped by context")
	}
}

func Test_ParentStop(t *testing.T) {
	var parent Job
	root.AddTask(&parent)
	var called atomic.Uint32
	var task Task
	checkCalled := func(expected uint32) {
		if count := called.Add(1); count != expected {
			t.Errorf("expected %d, got %d", expected, count)
		}
	}
	task.OnDispose(func() {
		checkCalled(1)
	})
	parent.OnDispose(func() {
		checkCalled(2)
	})
	parent.AddTask(&task)
	parent.Stop(ErrAutoStop)
	if !errors.Is(task.WaitStopped(), ErrAutoStop) {
		t.Errorf("expected task auto stop")
	}
}

func Test_ParentAutoStop(t *testing.T) {
	var parent Job
	root.AddTask(&parent)
	var called atomic.Uint32
	var task Task
	checkCalled := func(expected uint32) {
		if count := called.Add(1); count != expected {
			t.Errorf("expected %d, got %d", expected, count)
		}
	}
	task.OnDispose(func() {
		checkCalled(1)
	})
	parent.OnDispose(func() {
		checkCalled(2)
	})
	parent.AddTask(&task)
	time.AfterFunc(time.Second, func() {
		task.Stop(ErrTaskComplete)
	})
	if !errors.Is(parent.WaitStopped(), ErrAutoStop) {
		t.Errorf("expected task auto stop")
	}
}

func Test_Hooks(t *testing.T) {
	var called atomic.Uint32
	var task Task
	checkCalled := func(expected uint32) {
		if count := called.Add(1); count != expected {
			t.Errorf("expected %d, got %d", expected, count)
		}
	}
	task.OnStart(func() {
		checkCalled(1)
	})
	task.OnDispose(func() {
		checkCalled(3)
	})
	task.OnStart(func() {
		checkCalled(2)
	})
	task.OnDispose(func() {
		checkCalled(4)
	})
	task.Stop(ErrTaskComplete)
	root.AddTask(&task).WaitStopped()
}

//
//type DemoTask struct {
//	Task
//	file     *os.File
//	filePath string
//}
//
//func (d *DemoTask) Start() (err error) {
//	d.file, err = os.Open(d.filePath)
//	return
//}
//
//func (d *DemoTask) Run() (err error) {
//	_, err = d.file.Write([]byte("hello"))
//	return
//}
//
//func (d *DemoTask) Dispose() {
//	d.file.Close()
//}
//
//type HelloWorld struct {
//	DemoTask
//}
//
//func (h *HelloWorld) Run() (err error) {
//	_, err = h.file.Write([]byte("world"))
//	return nil
//}

//type HelloWorld struct {
//	Task
//}
//
//func (h *HelloWorld) Start() (err error) {
//	fmt.Println("Hello World")
//	return nil
//}
