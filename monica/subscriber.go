package monica

import (
	"context"
	"fmt"
	"github.com/langhuihui/monibuca/monica/avformat"
	"time"
)

type Subscriber interface {
	Send(*avformat.SendPacket) error
}

type SubscriberInfo struct {
	ID            string
	TotalDrop     int //总丢帧
	TotalPacket   int
	Type          string
	BufferLength  int
	SubscribeTime time.Time
}
type OutputStream struct {
	context.Context
	*Room
	SubscriberInfo
	SendHandler      func(*avformat.SendPacket) error
	Cancel           context.CancelFunc
	Sign             string
	VTSent           bool
	ATSent           bool
	VSentTime        uint32
	ASentTime        uint32
	packetQueue      chan *avformat.SendPacket
	dropCount        int
	OffsetTime       uint32
	firstScreenIndex int
}

func (s *OutputStream) IsClosed() bool {
	return s.Context != nil && s.Err() != nil
}

func (s *OutputStream) Close() {
	if s.Cancel != nil {
		s.Cancel()
	}
}
func (s *OutputStream) Play(streamPath string) (err error) {
	AllRoom.Get(streamPath).Subscribe(s)
	defer s.UnSubscribe(s)
	for {
		select {
		case <-s.Done():
			return s.Err()
		case p := <-s.packetQueue:
			if err = s.SendHandler(p); err != nil {
				s.Cancel() //此处为了使得IsClosed 返回true
				return
			}
			p.Recycle()
		}
	}
}
func (s *OutputStream) sendPacket(packet *avformat.AVPacket, timestamp uint32) {
	if !packet.IsAVCSequence && timestamp == 0 {
		timestamp = 1 //防止为0
	}
	s.TotalPacket++
	s.BufferLength = len(s.packetQueue)
	if s.dropCount > 0 {
		if packet.IsKeyFrame() {
			fmt.Printf("%s drop packet:%d\n", s.ID, s.dropCount)
			s.dropCount = 0 //退出丢包
		} else {
			s.dropCount++
			s.TotalDrop++
			return
		}
	}
	if s.BufferLength == cap(s.packetQueue) {
		s.dropCount++
		s.TotalDrop++
		packet.Recycle()
	} else if !s.IsClosed() {
		s.packetQueue <- avformat.NewSendPacket(packet, timestamp)
	}
}

func (s *OutputStream) sendVideo(video *avformat.AVPacket) error {
	isKF := video.IsKeyFrame()
	if s.VTSent {
		if s.FirstScreen == nil || s.firstScreenIndex == -1 {
			s.sendPacket(video, video.Timestamp-s.VSentTime+s.OffsetTime)
		} else if !isKF && s.firstScreenIndex < len(s.FirstScreen) {
			firstScreen := s.FirstScreen[s.firstScreenIndex]
			firstScreen.RefCount++
			s.VSentTime = firstScreen.Timestamp - s.FirstScreen[0].Timestamp
			s.sendPacket(firstScreen, s.VSentTime)
			video.Recycle() //回收当前数据
			s.firstScreenIndex++
		} else {
			s.firstScreenIndex = -1 //收到关键帧或者首屏缓冲已播完后退出首屏渲染模式
			s.OffsetTime += s.VSentTime
			s.VSentTime = video.Timestamp
			s.sendPacket(video, s.OffsetTime)
		}
		return nil
	}
	//非首屏渲染模式跳过开头的非关键帧
	if !isKF {
		if s.FirstScreen == nil {
			return nil
		}
	} else if s.FirstScreen != nil {
		s.firstScreenIndex = -1 //跳过首屏渲染
	}
	s.VTSent = true
	s.sendPacket(s.VideoTag, 0)
	s.VSentTime = video.Timestamp
	return s.sendVideo(video)
}
func (s *OutputStream) sendAudio(audio *avformat.AVPacket) error {
	if s.ATSent {
		if s.FirstScreen != nil && s.firstScreenIndex == -1 {
			audio.Recycle()
			return nil
		}
		s.sendPacket(audio, audio.Timestamp-s.ASentTime)
		return nil
	}
	s.ATSent = true
	s.sendPacket(s.AudioTag, 0)
	s.ASentTime = audio.Timestamp
	return s.sendAudio(audio)
}
