package m7s

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"time"

	"m7s.live/v5/pkg/codec"

	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
)

type PublisherState int

const (
	PublisherStateInit PublisherState = iota
	PublisherStateTrackAdded
	PublisherStateSubscribed
	PublisherStateWaitSubscriber
	PublisherStateDisposed
)

const (
	PublishTypePull      = "pull"
	PublishTypeServer    = "server"
	PublishTypeVod       = "vod"
	PublishTypeTransform = "transform"
	PublishTypeReplay    = "replay"
)

type AVTracks struct {
	*AVTrack
	util.Collection[reflect.Type, *AVTrack]
	sync.RWMutex
	baseTs time.Duration //from old publisher's lastTs
}

func (t *AVTracks) Set(track *AVTrack) {
	t.Lock()
	defer t.Unlock()
	t.AVTrack = track
	track.BaseTs = t.baseTs
	t.Add(track)
}

func (t *AVTracks) SetMinBuffer(start time.Duration) {
	if t.AVTrack == nil {
		return
	}
	t.AVTrack.BufferRange[0] = start
}

func (t *AVTracks) GetOrCreate(dataType reflect.Type) *AVTrack {
	t.Lock()
	defer t.Unlock()
	if track, ok := t.Get(dataType); ok {
		return track
	}
	if t.AVTrack == nil {
		return nil
	}
	return t.CreateSubTrack(dataType)
}

func (t *AVTracks) CheckTimeout(timeout time.Duration) bool {
	if t.AVTrack == nil || t.AVTrack.LastValue.WriteTime.IsZero() {
		return false
	}
	return time.Since(t.AVTrack.LastValue.WriteTime) > timeout
}

func (t *AVTracks) CreateSubTrack(dataType reflect.Type) (track *AVTrack) {
	track = NewAVTrack(dataType, t.AVTrack)
	track.WrapIndex = t.Length
	t.Add(track)
	return
}

func (t *AVTracks) Dispose() {
	t.Lock()
	defer t.Unlock()
	for track := range t.Range {
		track.Ready(ErrDisposed)
		if track == t.AVTrack || track.RingWriter != t.AVTrack.RingWriter {
			track.Dispose()
		}
	}
	t.AVTrack = nil
	t.Clear()
}

type Publisher struct {
	PubSubBase
	config.Publish
	State                  PublisherState
	Paused                 *util.Promise
	pauseTime              time.Time
	AudioTrack, VideoTrack AVTracks
	audioReady, videoReady *util.Promise
	TimeoutTimer           *time.Timer
	DataTrack              *DataTrack
	Subscribers            SubscriberCollection
	GOP                    int
	OnSeek                 func(time.Time)
	OnGetPosition          func() time.Time
	PullProxyConfig        *PullProxyConfig
	dropAfterTs            time.Duration
}

type PublishParam struct {
	Context      context.Context
	Audio, Video IAVFrame
	StreamPath   string
	Config       *config.Publish
}

func (p *Publisher) SubscriberRange(yield func(sub *Subscriber) bool) {
	p.Subscribers.Range(yield)
}

func (p *Publisher) GetKey() string {
	return p.StreamPath
}

func (p *Publisher) Start() (err error) {
	s := p.Plugin.Server
	if p.MaxCount > 0 && s.Streams.Length >= p.MaxCount {
		return ErrPublishMaxCount
	}
	p.Info("publish")
	p.processPullProxyOnStart()
	p.audioReady = util.NewPromiseWithTimeout(p, p.PublishTimeout)
	if !p.PubAudio {
		p.audioReady.Reject(ErrMuted)
	}
	p.videoReady = util.NewPromiseWithTimeout(p, p.PublishTimeout)
	if !p.PubVideo {
		p.videoReady.Reject(ErrMuted)
	}
	s.Waiting.WakeUp(p.StreamPath, p)
	p.processAliasOnStart()
	p.Plugin.Server.OnPublish(p)
	return
}

func (p *Publisher) Go() error {
	noDataCheck := time.NewTicker(time.Second * 5)
	defer noDataCheck.Stop()
	for {
		select {
		case <-p.TimeoutTimer.C:
			if p.Paused != nil {
				continue
			}
			switch p.State {
			case PublisherStateInit:
				if p.HasAudioTrack() || p.HasVideoTrack() {
					if p.Publish.IdleTimeout > 0 && time.Since(p.StartTime) > p.Publish.IdleTimeout {
						p.Stop(ErrPublishIdleTimeout)
					}
				} else {
					p.Stop(ErrPublishTimeout)
				}
			case PublisherStateSubscribed:
			case PublisherStateWaitSubscriber:
				if p.Publish.DelayCloseTimeout > 0 {
					p.Stop(ErrPublishDelayCloseTimeout)
				}
			}
		case <-noDataCheck.C:
			if p.Paused != nil {
				continue
			}
			if p.PubVideo && p.VideoTrack.CheckTimeout(p.PublishTimeout) {
				p.Error("video timeout", "writeTime", p.VideoTrack.LastValue.WriteTime)
				if !p.HasAudioTrack() {
					p.Stop(ErrPublishTimeout)
				}
				p.NoVideo()
			}
			if p.PubAudio && p.AudioTrack.CheckTimeout(p.PublishTimeout) {
				p.Error("audio timeout", "writeTime", p.AudioTrack.LastValue.WriteTime)
				if !p.HasVideoTrack() {
					p.Stop(ErrPublishTimeout)
				}
				p.NoAudio()
			}
		case <-p.Done():
			return p.Err()
		}
	}
}

func (p *Publisher) RemoveSubscriber(subscriber *Subscriber) {
	p.Subscribers.Remove(subscriber)
	p.Info("subscriber -1", "count", p.Subscribers.Length)
	if p.Plugin == nil {
		return
	}
	if subscriber.BufferTime == p.BufferTime && p.Subscribers.Length > 0 {
		p.BufferTime = slices.MaxFunc(p.Subscribers.Items, func(a, b *Subscriber) int {
			return int(a.BufferTime - b.BufferTime)
		}).BufferTime
	} else {
		p.BufferTime = p.Plugin.GetCommonConf().Publish.BufferTime
	}
	p.AudioTrack.SetMinBuffer(p.BufferTime)
	p.VideoTrack.SetMinBuffer(p.BufferTime)
	if p.State == PublisherStateSubscribed && p.Subscribers.Length == 0 {
		p.State = PublisherStateWaitSubscriber
		if p.DelayCloseTimeout > 0 {
			p.TimeoutTimer.Reset(p.DelayCloseTimeout)
		}
	}
}

func (p *Publisher) AddSubscriber(subscriber *Subscriber) {
	oldPublisher := subscriber.Publisher
	subscriber.Publisher = p
	if oldPublisher == nil {
		close(subscriber.waitPublishDone)
	} else {
		if subscriber.waitingPublish() {
			subscriber.Info("publisher recover", "pid", p.ID)
		} else {
			subscriber.Info("publisher changed", "prePid", oldPublisher.ID, "pid", p.ID)
		}
	}
	subscriber.waitStartTime = time.Time{}
	if p.Subscribers.AddUnique(subscriber) {
		p.Info("subscriber +1", "count", p.Subscribers.Length)
		if subscriber.BufferTime > p.BufferTime {
			p.BufferTime = subscriber.BufferTime
			p.AudioTrack.SetMinBuffer(p.BufferTime)
			p.VideoTrack.SetMinBuffer(p.BufferTime)
		}
		p.State = PublisherStateSubscribed
		if p.PublishTimeout > 0 {
			p.TimeoutTimer.Reset(p.PublishTimeout)
		}
	}
}

func (p *Publisher) writeAV(t *AVTrack, avFrame *AVFrame, codecCtxChanged bool, tracks *AVTracks) (err error) {
	t.AcceptFrame()
	if p.TraceEnabled() {
		frame := &t.Value
		codec := t.FourCC().String()
		p.Trace("write", "seq", frame.Sequence, "baseTs", int32(t.BaseTs/time.Millisecond), "ts0", uint32(avFrame.TS0/time.Millisecond), "ts", uint32(frame.Timestamp/time.Millisecond), "codec", codec, "size", frame.Size, "data", frame.Wraps[0].String())
	}
	// 处理子轨道
	if tracks.Length > 1 && tracks.IsReady() {
		for i, track := range tracks.Items[1:] {
			if track.ICodecCtx == nil {
				// 为新的子轨道初始化历史帧
				if tracks == &p.VideoTrack {
					// 视频轨道使用 IDRingList
					if t.IDRingList.Len() > 0 {
						for rf := t.IDRingList.Front().Value; rf != t.Ring; rf = rf.Next() {
							toFrame := track.NewFrame(&rf.Value)
							toSample := toFrame.GetSample()
							if track.ICodecCtx != nil {
								toSample.ICodecCtx = track.ICodecCtx
							}
							err = ConvertFrameType(rf.Value.Wraps[0], toFrame)
							if err != nil {
								track.ICodecCtx = nil
								return
							}
							track.ICodecCtx = toSample.ICodecCtx
							if track.ICodecCtx == nil {
								return ErrUnsupportCodec
							}
							rf.Value.Wraps = append(rf.Value.Wraps, toFrame)
						}
					}
				} else {
					// 音频轨道使用 GetOldestIDR
					if idr := tracks.GetOldestIDR(); idr != nil {
						for rf := idr; rf != t.Ring; rf = rf.Next() {
							toFrame := track.NewFrame(&rf.Value)
							toSample := toFrame.GetSample()
							if track.ICodecCtx != nil {
								toSample.ICodecCtx = track.ICodecCtx
							}
							err = ConvertFrameType(rf.Value.Wraps[0], toFrame)
							if err != nil {
								track.ICodecCtx = nil
								return
							}
							track.ICodecCtx = toSample.ICodecCtx
							if track.ICodecCtx == nil {
								return ErrUnsupportCodec
							}
							rf.Value.Wraps = append(rf.Value.Wraps, toFrame)
						}
					}
				}
			}

			// 处理当前帧的转换
			var toFrame IAVFrame
			if len(avFrame.Wraps) > i+1 {
				toFrame = avFrame.Wraps[i+1]
			} else {
				toFrame = track.NewFrame(avFrame)
				avFrame.Wraps = append(avFrame.Wraps, toFrame)
			}
			toSample := toFrame.GetSample()
			if codecCtxChanged {
				track.ICodecCtx = nil
			} else {
				toSample.ICodecCtx = track.ICodecCtx
			}
			err = ConvertFrameType(avFrame.Wraps[0], toFrame)
			track.ICodecCtx = toSample.ICodecCtx
			if track.ICodecCtx != nil {
				track.Ready(err)
			}
		}
	}
	if !t.Step() {
		err = ErrDisposed
	}
	return
}

func (p *Publisher) checkCodecChange(t *AVTrack) (codecCtxChanged bool, err error) {
	avFrame := &t.Value
	if t.Allocator == nil {
		t.Allocator = avFrame.GetAllocator()
	}
	err = avFrame.Wraps[0].CheckCodecChange()
	if err != nil {
		return
	}
	if avFrame.ICodecCtx == nil {
		err = ErrUnsupportCodec
		return
	}
	oldCodecCtx := t.ICodecCtx
	t.ICodecCtx = avFrame.ICodecCtx
	avFrame.TS0 = avFrame.Timestamp
	t.FixTimestamp(avFrame.Sample, p.Scale)
	codecCtxChanged = oldCodecCtx != t.ICodecCtx
	return
}

func (p *Publisher) nextVideo() (err error) {
	t := p.VideoTrack.AVTrack
	defer func() {
		if err == nil {
			t.SpeedControl(p.Speed)
		} else if t != nil {
			t.Value.Reset()
		}
	}()
	if err = p.Err(); err != nil {
		return
	}
	avFrame := &t.Value
	oldCodecCtx := t.ICodecCtx
	var codecCtxChanged bool
	codecCtxChanged, err = p.checkCodecChange(t)
	if err != nil {
		return err
	}
	if codecCtxChanged && oldCodecCtx != nil {
		oldWidth, oldHeight := oldCodecCtx.(IVideoCodecCtx).Width(), oldCodecCtx.(IVideoCodecCtx).Height()
		newWidth, newHeight := t.ICodecCtx.(IVideoCodecCtx).Width(), t.ICodecCtx.(IVideoCodecCtx).Height()
		if oldWidth != newWidth || oldHeight != newHeight {
			p.Info("video resolution changed", "oldWidth", oldWidth, "oldHeight", oldHeight, "newWidth", newWidth, "newHeight", newHeight)
		}
	}
	var idr *util.Ring[AVFrame]
	if t.IDRingList.Len() > 0 {
		idr = t.IDRingList.Back().Value
		if p.Speed != 1 && t.CheckIfNeedDropFrame(p.MaxFPS) {
			p.dropAfterTs = t.LastTs
			return ErrSkip
		} else {
			p.dropAfterTs = 0
		}
	}

	if avFrame.IDR {
		if !t.IsReady() {
			t.Ready(nil)
		} else if idr != nil {
			p.GOP = int(t.Value.Sequence - idr.Value.Sequence)
		} else {
			p.GOP = 0
		}
		if p.AudioTrack.Length > 0 {
			p.AudioTrack.PushIDR()
		}
	}
	return p.writeAV(t, avFrame, codecCtxChanged, &p.VideoTrack)
}

func (p *Publisher) nextAudio() (err error) {
	t := p.AudioTrack.AVTrack
	defer func() {
		if err == nil {
			t.SpeedControl(p.Speed)
		} else if t != nil {
			t.Value.Recycle()
		}
	}()
	if err = p.Err(); err != nil {
		return
	}
	avFrame := &t.Value
	var codecCtxChanged bool
	codecCtxChanged, err = p.checkCodecChange(t)
	if err != nil {
		return err
	}
	// 根据丢帧率进行音频帧丢弃
	if p.dropAfterTs > 0 {
		if t.LastTs > p.dropAfterTs {
			return ErrSkip
		}
	}
	if !t.IsReady() {
		t.Ready(nil)
	}
	return p.writeAV(t, avFrame, codecCtxChanged, &p.AudioTrack)
}

func (p *Publisher) WriteData(data IDataFrame) (err error) {
	for subscriber := range p.SubscriberRange {
		if subscriber.DataChannel == nil {
			continue
		}
		select {
		case subscriber.DataChannel <- data:
		default:
			p.Warn("subscriber channel full", "subscriber", subscriber.ID)
		}
	}
	return nil
}

func (p *Publisher) GetAudioCodecCtx() (ctx codec.ICodecCtx) {
	if p.HasAudioTrack() {
		return p.AudioTrack.ICodecCtx
	}
	return nil
}

func (p *Publisher) GetVideoCodecCtx() (ctx codec.ICodecCtx) {
	if p.HasVideoTrack() {
		return p.VideoTrack.ICodecCtx
	}
	return nil
}

func (p *Publisher) GetAudioTrack(dataType reflect.Type) (t *AVTrack) {
	return p.AudioTrack.GetOrCreate(dataType)
}

func (p *Publisher) GetVideoTrack(dataType reflect.Type) (t *AVTrack) {
	return p.VideoTrack.GetOrCreate(dataType)
}

func (p *Publisher) HasAudioTrack() bool {
	return p.AudioTrack.Length > 0
}

func (p *Publisher) HasVideoTrack() bool {
	return p.VideoTrack.Length > 0
}

func (p *Publisher) Dispose() {
	s := p.Plugin.Server
	if p.Paused != nil {
		p.Paused.Reject(p.StopReason())
	}
	p.processAliasOnDispose()
	p.AudioTrack.Dispose()
	p.VideoTrack.Dispose()
	p.Info("unpublish", "remain", s.Streams.Length, "reason", p.StopReason())
	p.State = PublisherStateDisposed
	p.processPullProxyOnDispose()
}

func (p *Publisher) TransferSubscribers(newPublisher *Publisher) {
	p.Info("transfer subscribers", "newPublisher", newPublisher.ID, "newStreamPath", newPublisher.StreamPath)
	var remain SubscriberCollection
	for subscriber := range p.SubscriberRange {
		if subscriber.Type != SubscribeTypeServer {
			remain.Add(subscriber)
		} else {
			newPublisher.AddSubscriber(subscriber)
		}
	}
	p.Subscribers = remain
	p.BufferTime = p.Plugin.GetCommonConf().Publish.BufferTime
	p.AudioTrack.SetMinBuffer(p.BufferTime)
	p.VideoTrack.SetMinBuffer(p.BufferTime)
	if p.State == PublisherStateSubscribed {
		p.State = PublisherStateWaitSubscriber
		if p.DelayCloseTimeout > 0 {
			p.TimeoutTimer.Reset(p.DelayCloseTimeout)
		}
	}
}

func (p *Publisher) takeOver(old *Publisher) {
	if old.HasAudioTrack() {
		p.AudioTrack.baseTs = old.AudioTrack.LastTs
	}
	if old.HasVideoTrack() {
		p.VideoTrack.baseTs = old.VideoTrack.LastTs
	}
	old.Stop(ErrKick)
	p.Info("takeOver", "old", old.ID)
	if old.Subscribers.Length > 0 {
		p.Info(fmt.Sprintf("subscriber +%d", old.Subscribers.Length))
		for subscriber := range old.SubscriberRange {
			subscriber.Publisher = p
			if subscriber.BufferTime > p.BufferTime {
				p.BufferTime = subscriber.BufferTime
			}
		}
	}
	old.AudioTrack.Dispose()
	old.VideoTrack.Dispose()
	old.Subscribers = SubscriberCollection{}
}

func (p *Publisher) WaitTrack(audio, video bool) error {
	var v, a = ErrNoTrack, ErrNoTrack
	// wait any track
	if p.PubAudio && p.PubVideo && !audio && !video {
		select {
		case <-p.videoReady.Done():
			v = context.Cause(p.videoReady.Context)
			if errors.Is(v, util.ErrResolve) {
				v = nil
			}
		case <-p.audioReady.Done():
			v = context.Cause(p.audioReady.Context)
			if errors.Is(v, util.ErrResolve) {
				v = nil
			}
		}
	} else {
		// need wait video
		if p.PubVideo && video {
			v = p.videoReady.Await()
		}
		// need wait audio
		if p.PubAudio && audio {
			a = p.audioReady.Await()
		}
	}
	if v != nil && a != nil {
		return ErrNoTrack
	}
	return nil
}

func (p *Publisher) NoVideo() {
	p.PubVideo = false
	if p.videoReady != nil {
		p.videoReady.Reject(ErrMuted)
	}
}

func (p *Publisher) NoAudio() {
	p.PubAudio = false
	if p.audioReady != nil {
		p.audioReady.Reject(ErrMuted)
	}
}

func (p *Publisher) Pause() {
	if p.Paused != nil {
		return
	}
	p.Paused = util.NewPromise(p)
	p.pauseTime = time.Now()
}

func (p *Publisher) Resume() {
	if p.Paused == nil {
		return
	}
	p.Paused.Resolve()
	p.Paused = nil
	if p.HasVideoTrack() {
		p.VideoTrack.AddPausedTime(time.Since(p.pauseTime))
	}
	if p.HasAudioTrack() {
		p.AudioTrack.AddPausedTime(time.Since(p.pauseTime))
	}
}

func (p *Publisher) Seek(ts time.Time) {
	p.Info("seek", "time", ts)
	if p.OnSeek != nil {
		p.OnSeek(ts)
	}
}

func (p *Publisher) GetPosition() (t time.Time) {
	if p.OnGetPosition != nil {
		return p.OnGetPosition()
	}
	return
}

type PublishAudioWriter[A IAVFrame] struct {
	AudioFrame A
	*Publisher
	*util.ScalableMemoryAllocator
	audioTrack *AVTrack
}

func NewPublishAudioWriter[A IAVFrame](puber *Publisher, allocator *util.ScalableMemoryAllocator) *PublishAudioWriter[A] {
	if !puber.PubAudio {
		return nil
	}
	pw := &PublishAudioWriter[A]{
		Publisher:               puber,
		ScalableMemoryAllocator: allocator,
	}
	t := pw.audioTrack
	if t == nil {
		var tmp A
		t = NewAVTrack(reflect.TypeOf(tmp), pw.Logger.With("track", "audio"), &pw.Publish, pw.audioReady)
		pw.AudioTrack.Set(t)
	}
	pw.audioTrack = t
	pw.AudioFrame = pw.getAudioFrameToWrite()
	return pw
}

func (pw *PublishAudioWriter[A]) getAudioFrameToWrite() (frame A) {
	if !pw.PubAudio || pw.audioTrack == nil {
		return
	}
	t := pw.audioTrack
	avFrame := &t.Value
	if avFrame.Sample == nil {
		avFrame.Wraps = append(avFrame.Wraps, t.NewFrame(avFrame))
	}
	avFrame.ICodecCtx = t.ICodecCtx
	frame = avFrame.Wraps[0].(A)
	frame.GetSample().SetAllocator(pw.ScalableMemoryAllocator)
	return
}

func (pw *PublishAudioWriter[A]) NextAudio() (err error) {
	if err = pw.nextAudio(); err != nil {
		if err == ErrSkip {
			return nil
		}
		return
	}
	pw.AudioFrame = pw.getAudioFrameToWrite()
	return
}

type PublishVideoWriter[V IAVFrame] struct {
	VideoFrame V
	*Publisher
	*util.ScalableMemoryAllocator
	videoTrack *AVTrack
}

func NewPublishVideoWriter[V IAVFrame](puber *Publisher, allocator *util.ScalableMemoryAllocator) *PublishVideoWriter[V] {
	if !puber.PubVideo {
		return nil
	}
	pw := &PublishVideoWriter[V]{
		Publisher:               puber,
		ScalableMemoryAllocator: allocator,
	}
	t := pw.videoTrack
	if t == nil {
		var tmp V
		t = NewAVTrack(reflect.TypeOf(tmp), pw.Logger.With("track", "video"), &pw.Publish, pw.videoReady)
		pw.VideoTrack.Set(t)
	}
	pw.videoTrack = t
	pw.VideoFrame = pw.getVideoFrameToWrite()
	return pw
}

func (pw *PublishVideoWriter[V]) getVideoFrameToWrite() (frame V) {
	if !pw.PubVideo || pw.videoTrack == nil {
		return
	}
	t := pw.videoTrack
	avFrame := &t.Value
	if avFrame.Sample == nil {
		avFrame.Wraps = append(avFrame.Wraps, t.NewFrame(avFrame))
	}
	avFrame.ICodecCtx = t.ICodecCtx
	frame = avFrame.Wraps[0].(V)
	frame.GetSample().SetAllocator(pw.ScalableMemoryAllocator)
	return
}

func (pw *PublishVideoWriter[V]) NextVideo() (err error) {
	if err = pw.nextVideo(); err != nil {
		if err == ErrSkip {
			return nil
		}
		return
	}
	pw.VideoFrame = pw.getVideoFrameToWrite()
	return
}

type PublishWriter[A IAVFrame, V IAVFrame] struct {
	*PublishAudioWriter[A]
	*PublishVideoWriter[V]
}

func NewPublisherWriter[A IAVFrame, V IAVFrame](puber *Publisher, allocator *util.ScalableMemoryAllocator) *PublishWriter[A, V] {
	return &PublishWriter[A, V]{
		PublishAudioWriter: NewPublishAudioWriter[A](puber, allocator),
		PublishVideoWriter: NewPublishVideoWriter[V](puber, allocator),
	}
}
