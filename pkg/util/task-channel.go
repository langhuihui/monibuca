package util

import (
	"time"
)

type ChannelTask struct {
	Task
	SignalChan any
}

func (*ChannelTask) GetTaskType() TaskType {
	return TASK_TYPE_CHANNEL
}

func (t *ChannelTask) GetSignal() any {
	return t.SignalChan
}

func (t *ChannelTask) Tick(any) {
}

type TickTask struct {
	ChannelTask
	Ticker *time.Ticker
}

func (t *TickTask) GetTickInterval() time.Duration {
	return time.Second
}

func (t *TickTask) Start() (err error) {
	t.Ticker = time.NewTicker(t.handler.(interface{ GetTickInterval() time.Duration }).GetTickInterval())
	t.SignalChan = t.Ticker.C
	return
}

func (t *TickTask) Dispose() {
	t.Ticker.Stop()
}
