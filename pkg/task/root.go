package task

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

type OSSignal struct {
	ChannelTask
	root interface {
		Shutdown()
	}
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
	m.Context = context.Background()
	m.handler = m
	m.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	m.AddTask(&OSSignal{})
}

func (m *RootManager[K, T]) Shutdown() {
	m.Stop(ErrExit)
	m.dispose()
}
