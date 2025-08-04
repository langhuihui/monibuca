package task

import (
	"time"
)

type ITickTask interface {
	IChannelTask
	GetTickInterval() time.Duration
	GetTicker() *time.Ticker
}

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

func (t *TickTask) GetTicker() *time.Ticker {
	return t.Ticker
}

func (t *TickTask) GetTickInterval() time.Duration {
	return time.Second
}

func (t *TickTask) Start() (err error) {
	t.Ticker = time.NewTicker(t.handler.(ITickTask).GetTickInterval())
	t.SignalChan = t.Ticker.C
	t.OnStop(func() {
		t.Ticker.Reset(time.Millisecond)
	})
	return
}

type AsyncTickTask struct {
	TickTask
}

func (t *AsyncTickTask) GetSignal() any {
	return t.Task.GetSignal()
}

func (t *AsyncTickTask) Go() error {
	t.handler.(ITickTask).Tick(nil)
	for {
		select {
		case c := <-t.Ticker.C:
			t.handler.(ITickTask).Tick(c)
		case <-t.Done():
			return nil
		}
	}
}
