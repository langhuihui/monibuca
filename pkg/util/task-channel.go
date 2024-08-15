package util

import "reflect"

type ChannelTask struct {
	Task
	channel  reflect.Value
	callback reflect.Value
}

func (t *ChannelTask) GetTaskType() string {
	return "channel"
}

func (t *ChannelTask) getSignal() reflect.Value {
	return t.channel
}

func (t *ChannelTask) tick(signal reflect.Value) {
	t.callback.Call([]reflect.Value{signal})
}
