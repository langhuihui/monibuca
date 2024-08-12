package pkg

import "reflect"

type ChannelTask struct {
	Task
	channel  reflect.Value
	callback reflect.Value
}

func (t *ChannelTask) start() (reflect.Value, error) {
	return t.channel, nil
}

func (t *ChannelTask) tick(signal reflect.Value) {
	t.callback.Call([]reflect.Value{signal})
}
