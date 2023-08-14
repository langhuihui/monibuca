package test

import (
	"fmt"
	"testing"
	"time"
)

// TestPubAndSub 测试发布和订阅
func TestPubAndSub(t *testing.T) {
	t.Cleanup(FreeEngine)
	UseEngine()
	t.Run("publish", func(t *testing.T) {
		t.Parallel()
		var pub UnitTestPublisher
		unitTestPlugin.Publish("test/001", &pub)
	})
	t.Run("subscribe", func(t *testing.T) {
		t.Parallel()
		var sub UnitTestSubsciber
		sub.tb = t
		err := unitTestPlugin.Subscribe("test/001", &sub)
		if err != nil {
			t.Fatal(err)
		} else {
			sub.PlayRaw()
		}
	})
}

func BenchmarkPubAndSub(b *testing.B) {
	b.Cleanup(FreeEngine)
	UseEngine()
	for i := 0; i < 10; i++ {
		i := i
		go func(i int) {
			var pub UnitTestPublisher
			unitTestPlugin.Publish(fmt.Sprintf("testb/%d", i), &pub)
		}(i)
		go b.RunParallel(func(pb *testing.PB) {
			var sub UnitTestSubsciber
			sub.tb = b
			err := unitTestPlugin.Subscribe(fmt.Sprintf("testb/%d", i), &sub)
			if err != nil {
				// b.Fatal(err)
			} else {
				sub.PlayRaw()
			}
		})
	}
	time.Sleep(time.Second * 10)
}
