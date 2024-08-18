package util

import "reflect"

type ChannelTask struct {
	Task
	channel  reflect.Value
	callback reflect.Value
}

func (*ChannelTask) GetTaskType() string {
	return "channel"
}

func (*ChannelTask) GetTaskTypeID() byte {
	return 3
}

func (t *ChannelTask) getSignal() reflect.Value {
	return t.channel
}

func (t *ChannelTask) tick(signal reflect.Value) {
	t.callback.Call([]reflect.Value{signal})
}
