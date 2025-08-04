package task

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type shutdown interface {
	Shutdown()
}

type OSSignal struct {
	ChannelTask
	root shutdown
}

func (o *OSSignal) Start() error {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	o.SignalChan = signalChan
	o.OnStop(func() {
		signal.Stop(signalChan)
		close(signalChan)
	})
	return nil
}

func (o *OSSignal) Tick(any) {
	println("OSSignal Tick")
	go o.root.Shutdown()
}

type RootManager[K comparable, T ManagerItem[K]] struct {
	WorkCollection[K, T]
}

func (m *RootManager[K, T]) Init() {
	m.parentCtx = context.Background()
	m.reset()
	m.handler = m
	m.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	m.StartTime = time.Now()
	m.AddTask(&OSSignal{root: m}).WaitStarted()
	m.state = TASK_STATE_STARTED
}

func (m *RootManager[K, T]) Shutdown() {
	m.Stop(ErrExit)
	m.dispose()
}
