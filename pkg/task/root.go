package task

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
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
	return nil
}

func (o *OSSignal) Tick(any) {
	go o.root.Shutdown()
}

type RootManager[K comparable, T ManagerItem[K]] struct {
	Manager[K, T]
}

func (m *RootManager[K, T]) Init() {
	m.Context, m.CancelCauseFunc = context.WithCancelCause(context.Background())
	m.handler = m
	m.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	m.state = TASK_STATE_STARTED
	m.AddTask(&OSSignal{root: m})
}

func (m *RootManager[K, T]) Shutdown() {
	m.Stop(ErrExit)
	m.dispose()
}
