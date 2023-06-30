package test

import (
	"testing"
	"time"

	. "m7s.live/engine/v4"
)

type SlowSubsciber struct {
	Subscriber
}

func (s *SlowSubsciber) OnEvent(event any) {
	switch event.(type) {
	case AudioFrame:
	case VideoFrame:
		// 模拟慢消费，导致长时间占用后被发布者移除
		time.Sleep(1000 * time.Millisecond)
	default:
		s.Subscriber.OnEvent(event)
	}
}

func TestSlowSubscriber(t *testing.T) {
	t.Cleanup(FreeEngine)
	UseEngine()
	var pub UnitTestPublisher
	unitTestPlugin.Publish("test/slow", &pub)
	var suber SlowSubsciber
	unitTestPlugin.Subscribe("test/slow", &suber)
	suber.PlayRaw()
}
