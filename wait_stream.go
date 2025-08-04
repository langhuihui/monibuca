package m7s

import (
	"log/slog"
	"time"

	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/util"
)

type WaitManager struct {
	*slog.Logger
	util.Collection[string, *WaitStream]
}

func (w *WaitManager) Wait(subscriber *Subscriber) *WaitStream {
	subscriber.waitStartTime = time.Now()
	if subscriber.Publisher != nil {
		subscriber.Info("publisher gone", "pid", subscriber.Publisher.ID)
	}
	if waiting, ok := w.Get(subscriber.StreamPath); ok {
		waiting.Add(subscriber)
		return waiting
	} else {
		waiting := &WaitStream{
			StreamPath: subscriber.StreamPath,
		}
		w.Set(waiting)
		waiting.Add(subscriber)
		return waiting
	}
}

func (w *WaitManager) WakeUp(streamPath string, publisher *Publisher) {
	if waiting, ok := w.Get(streamPath); ok {
		for subscriber := range waiting.Range {
			publisher.AddSubscriber(subscriber)
		}
		waiting.Clear()
		publisher.OnDispose(func() {
			if waiting.Length == 0 {
				w.Remove(waiting)
			}
		})
		// w.Remove(waiting)
	}
}

func (w *WaitManager) checkTimeout() {
	for waits := range w.Range {
		for sub := range waits.Range {
			if time.Since(sub.waitStartTime) > max(sub.WaitTimeout, sub.BufferTime) {
				sub.Stop(ErrSubscribeTimeout)
			}
		}
	}
}

func (w *WaitManager) Leave(s *Subscriber) {
	if waitStream, ok := w.Get(s.StreamPath); ok {
		waitStream.Remove(s)
	}
}
