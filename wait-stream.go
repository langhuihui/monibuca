package m7s

import (
	"log/slog"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
)

type WaitManager struct {
	*slog.Logger
	util.Collection[string, *WaitStream]
}

func (w *WaitManager) Wait(subscriber *Subscriber) *WaitStream {
	subscriber.Publisher = nil
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
		w.Remove(waiting)
	}
}

func (w *WaitManager) checkTimeout() {
	for waits := range w.Range {
		for sub := range waits.Range {
			select {
			case <-sub.TimeoutTimer.C:
				sub.Stop(ErrSubscribeTimeout)
			default:
			}
		}
	}
}

func (w *WaitManager) Leave(s *Subscriber) {
	if waitStream, ok := w.Get(s.StreamPath); ok {
		waitStream.Remove(s)
	}
}
