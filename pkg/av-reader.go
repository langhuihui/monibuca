package pkg

import (
	"context"
	"log/slog"
	"time"
)

const (
	READSTATE_INIT = iota
	READSTATE_FIRST
	READSTATE_NORMAL
)
const (
	SUBMODE_REAL = iota
	SUBMODE_NOJUMP
	SUBMODE_BUFFER
)

type AVRingReader struct {
	RingReader
	mode         int
	Track        *AVTrack
	State        byte
	FirstSeq     uint32
	StartTs      time.Duration
	FirstTs      time.Duration
	SkipTs       time.Duration //ms
	beforeJump   time.Duration
	LastCodecCtx ICodecCtx
	startTime    time.Time
	AbsTime      uint32
	Delay        uint32
	*slog.Logger
}

func (r *AVRingReader) DecConfChanged() bool {
	return r.LastCodecCtx != r.Track.ICodecCtx
}

func NewAVRingReader(t *AVTrack) *AVRingReader {
	t.Debug("create reader")
	<-t.Ready.Done()
	t.Info("reader +1", "count", t.ReaderCount.Add(1))
	return &AVRingReader{
		Track: t,
	}
}

func (r *AVRingReader) readFrame() (err error) {
	err = r.ReadNext()
	if err != nil {
		return err
	}
	// 超过一半的缓冲区大小，说明Reader太慢，需要丢帧
	if r.mode != SUBMODE_BUFFER && r.State == READSTATE_NORMAL && r.Track.LastValue.Sequence-r.Value.Sequence > uint32(r.Track.Size/2) {
		idr := r.Track.IDRing.Load()
		if idr != nil && idr.Value.Sequence > r.Value.Sequence {
			r.Warn("reader too slow", "lastSeq", r.Track.LastValue.Sequence, "seq", r.Value.Sequence)
			return r.Read(idr)
		}
	}
	return
}

func (r *AVRingReader) ReadFrame(mode int) (err error) {
	r.mode = mode
	switch r.State {
	case READSTATE_INIT:
		r.Info("start read", "mode", mode)
		startRing := r.Track.Ring
		idr := r.Track.IDRing.Load()
		if idr != nil {
			startRing = idr
		} else {
			r.Warn("no IDRring", "track", r.Track.FourCC().String())
		}
		switch mode {
		case SUBMODE_REAL:
			if idr != nil {
				r.State = READSTATE_FIRST
			} else {
				r.State = READSTATE_NORMAL
			}
		case SUBMODE_NOJUMP:
			r.State = READSTATE_NORMAL
		case SUBMODE_BUFFER:
			if idr := r.Track.HistoryRing.Load(); idr != nil {
				startRing = idr
			}
			r.State = READSTATE_NORMAL
		}
		if err = r.StartRead(startRing); err != nil {
			return
		}
		r.startTime = time.Now()
		if r.FirstTs == 0 {
			r.FirstTs = r.Value.Timestamp
		}
		r.SkipTs = r.FirstTs - r.StartTs
		r.FirstSeq = r.Value.Sequence
		r.Info("first frame read", "firstTs", r.FirstTs, "firstSeq", r.FirstSeq)
	case READSTATE_FIRST:
		if idr := r.Track.IDRing.Load(); idr.Value.Sequence != r.FirstSeq {
			if err = r.Read(idr); err != nil {
				return
			}
			r.SkipTs = r.Value.Timestamp - r.beforeJump - r.StartTs - 10*time.Millisecond
			r.Info("jump", "skipSeq", idr.Value.Sequence-r.FirstSeq, "skipTs", r.SkipTs)
			r.State = READSTATE_NORMAL
		} else {
			if err = r.readFrame(); err != nil {
				return
			}
			r.beforeJump = r.Value.Timestamp - r.FirstTs
			// 防止过快消费
			if fast := r.beforeJump - time.Since(r.startTime); fast > 0 && fast < time.Second {
				time.Sleep(fast)
			}
		}
	case READSTATE_NORMAL:
		if err = r.readFrame(); err != nil {
			return
		}
	}
	r.AbsTime = uint32((r.Value.Timestamp - r.SkipTs).Milliseconds())
	if r.AbsTime == 0 {
		r.AbsTime = 1
	}
	r.Delay = uint32(r.Track.LastValue.Sequence - r.Value.Sequence)
	r.Log(context.TODO(), TraceLevel, r.Track.FourCC().String(), "delay", r.Delay)
	return
}

// func (r *AVRingReader) GetPTS32() uint32 {
// 	return uint32((r.Value.Raw.PTS - r.SkipTs*90/time.Millisecond))
// }

// func (r *AVRingReader) GetDTS32() uint32 {
// 	return uint32((r.Value.DTS - r.SkipTs*90/time.Millisecond))
// }

func (r *AVRingReader) ResetAbsTime() {
	r.SkipTs = r.Value.Timestamp
	r.AbsTime = 1
}
