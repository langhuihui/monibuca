package monica

import (
	"context"
	"github.com/langhuihui/monibuca/monica/avformat"
	"github.com/langhuihui/monibuca/monica/pool"
	"log"
	"sync"
	"time"
)

var (
	AllRoom   = Collection{}
	roomCtxBg = context.Background()
)

type Collection struct {
	sync.Map
}

func (c *Collection) Get(name string) (result *Room) {
	item, loaded := AllRoom.LoadOrStore(name, &Room{
		Subscribers: make(map[string]*OutputStream),
		Control:     make(chan interface{}),
		VideoChan:   make(chan *pool.AVPacket, 1),
		AudioChan:   make(chan *pool.AVPacket, 1),
	})
	result = item.(*Room)
	if !loaded {
		result.StreamPath = name
		result.Context, result.Cancel = context.WithCancel(roomCtxBg)
		go result.Run()
	}
	return
}

type Room struct {
	context.Context
	Publisher
	RoomInfo
	Control      chan interface{}
	Cancel       context.CancelFunc
	Subscribers  map[string]*OutputStream // 订阅者
	VideoTag     *pool.AVPacket           // 每个视频包都是这样的结构,区别在于Payload的大小.FMS在发送AVC sequence header,需要加上 VideoTags,这个tag 1个字节(8bits)的数据
	AudioTag     *pool.AVPacket           // 每个音频包都是这样的结构,区别在于Payload的大小.FMS在发送AAC sequence header,需要加上 AudioTags,这个tag 1个字节(8bits)的数据
	FirstScreen  []*pool.AVPacket
	AudioChan    chan *pool.AVPacket
	VideoChan    chan *pool.AVPacket
	UseTimestamp bool //是否采用数据包中的时间戳
}

type RoomInfo struct {
	StreamPath     string
	StartTime      time.Time
	SubscriberInfo []*SubscriberInfo
	Type           string
	VideoInfo      struct {
		PacketCount int
		CodecID     byte
		SPSInfo     avformat.SPSInfo
	}
	AudioInfo struct {
		PacketCount int
		SoundFormat byte //4bit
		SoundRate   int  //2bit
		SoundSize   byte //1bit
		SoundType   byte //1bit
	}
}
type UnSubscribeCmd struct {
	*OutputStream
}
type SubscribeCmd struct {
	*OutputStream
}
type ChangeRoomCmd struct {
	*OutputStream
	NewRoom *Room
}

func (r *Room) onClosed() {
	log.Printf("room destoryed :%s", r.StreamPath)
	AllRoom.Delete(r.StreamPath)
	if r.Publisher != nil {
		r.OnClosed()
	}
}
func (r *Room) Subscribe(s *OutputStream) {
	s.Room = r
	if r.Err() == nil {
		s.SubscribeTime = time.Now()
		log.Printf("subscribe :%s %s,to room %s", s.Type, s.ID, r.StreamPath)
		s.packetQueue = make(chan *pool.SendPacket, 1024)
		s.Context, s.Cancel = context.WithCancel(r)
		s.Control <- &SubscribeCmd{s}
	}
}

func (r *Room) UnSubscribe(s *OutputStream) {
	if r.Err() == nil {
		r.Control <- &UnSubscribeCmd{s}
	}
}
func (r *Room) Run() {
	log.Printf("room create :%s", r.StreamPath)
	defer r.onClosed()
	update := time.NewTicker(time.Second)
	defer update.Stop()
	for {
		select {
		case <-r.Done():
			return
		case <-update.C:
			if Summary.Running() {
				r.SubscriberInfo = make([]*SubscriberInfo, len(r.Subscribers))
				i := 0
				for _, v := range r.Subscribers {
					r.SubscriberInfo[i] = &v.SubscriberInfo
					i++
				}
			}
		case s := <-r.Control:
			switch v := s.(type) {
			case *UnSubscribeCmd:
				delete(r.Subscribers, v.ID)
				log.Printf("%s subscriber %s removed remains:%d", r.StreamPath, v.ID, len(r.Subscribers))
				if len(r.Subscribers) == 0 && r.Publisher == nil {
					r.Cancel()
				}
			case *SubscribeCmd:
				if _, ok := r.Subscribers[v.ID]; !ok {
					r.Subscribers[v.ID] = v.OutputStream
					log.Printf("%s subscriber %s added remains:%d", r.StreamPath, v.ID, len(r.Subscribers))
					OnSubscribeHooks.Trigger(v.OutputStream)
				}
			case *ChangeRoomCmd:
				if _, ok := v.NewRoom.Subscribers[v.ID]; !ok {
					delete(r.Subscribers, v.ID)
					v.NewRoom.Subscribe(v.OutputStream)
					if len(r.Subscribers) == 0 && r.Publisher == nil {
						r.Cancel()
					}
				}
			}
		case audio := <-r.AudioChan:
			for _, v := range r.Subscribers {
				v.sendAudio(audio)
			}
		case video := <-r.VideoChan:
			for _, v := range r.Subscribers {
				v.sendVideo(video)
			}
		}
	}
}
func (r *Room) PushAudio(audio *pool.AVPacket) {
	if audio.Payload[0] == 0xFF && (audio.Payload[1]&0xF0) == 0xF0 {
		audio.IsADTS = true
		r.AudioTag = audio
	} else if r.AudioTag == nil {
		audio.IsAACSequence = true
		r.AudioTag = audio
		tmp := audio.Payload[0]                                                // 第一个字节保存着音频的相关信息
		if r.AudioInfo.SoundFormat = tmp >> 4; r.AudioInfo.SoundFormat == 10 { //真的是AAC的话，后面有一个字节的详细信息
			//0 = AAC sequence header，1 = AAC raw。
			if aacPacketType := audio.Payload[1]; aacPacketType == 0 {
				config1 := audio.Payload[2]
				config2 := audio.Payload[3]
				//audioObjectType = (config1 & 0xF8) >> 3
				// 1 AAC MAIN 	ISO/IEC 14496-3 subpart 4
				// 2 AAC LC 	ISO/IEC 14496-3 subpart 4
				// 3 AAC SSR 	ISO/IEC 14496-3 subpart 4
				// 4 AAC LTP 	ISO/IEC 14496-3 subpart 4
				r.AudioInfo.SoundRate = avformat.SamplingFrequencies[((config1&0x7)<<1)|(0x90>>7)]
				r.AudioInfo.SoundType = (config2 >> 3) & 0x0F //声道
				//frameLengthFlag = (config2 >> 2) & 0x01
				//dependsOnCoreCoder = (config2 >> 1) & 0x01
				//extensionFlag = config2 & 0x01
			}
		} else {
			r.AudioInfo.SoundRate = avformat.SoundRate[(tmp&0x0c)>>2] // 采样率 0 = 5.5 kHz or 1 = 11 kHz or 2 = 22 kHz or 3 = 44 kHz
			r.AudioInfo.SoundSize = (tmp & 0x02) >> 1                 // 采样精度 0 = 8-bit samples or 1 = 16-bit samples
			r.AudioInfo.SoundType = tmp & 0x01                        // 0 单声道，1立体声
		}
		return
	}
	audio.RefCount = len(r.Subscribers)
	if !r.UseTimestamp {
		audio.Timestamp = uint32(time.Since(r.StartTime) / time.Millisecond)
	}
	r.AudioInfo.PacketCount++
	r.AudioChan <- audio
}
func (r *Room) setH264Info(video *pool.AVPacket) {
	r.VideoTag = video
	info := avformat.AVCDecoderConfigurationRecord{}
	//0:codec,1:IsAVCSequence,2~4:compositionTime
	if _, err := info.Unmarshal(video.Payload[5:]); err == nil {
		r.VideoInfo.SPSInfo, err = avformat.ParseSPS(info.SequenceParameterSetNALUnit)
	}
}
func (r *Room) PushVideo(video *pool.AVPacket) {
	video.VideoFrameType = video.Payload[0] >> 4  // 帧类型 4Bit, H264一般为1或者2
	r.VideoInfo.CodecID = video.Payload[0] & 0x0f // 编码类型ID 4Bit, JPEG, H263, AVC...
	video.IsAVCSequence = video.VideoFrameType == 1 && video.Payload[1] == 0
	if r.VideoTag == nil {
		if video.IsAVCSequence {
			r.setH264Info(video)
		} else {
			log.Println("no AVCSequence")
		}
	} else {
		//更换AVCSequence
		if video.IsAVCSequence {
			r.setH264Info(video)
		}
		if r.FirstScreen != nil {
			if video.IsKeyFrame() {
				for _, cache := range r.FirstScreen { //清空队列
					cache.Recycle()
				}
				r.FirstScreen = r.FirstScreen[:0]
			}
			r.FirstScreen = append(r.FirstScreen, video)
			video.RefCount = len(r.Subscribers) + 1
		} else {
			video.RefCount = len(r.Subscribers)
		}
		if !r.UseTimestamp {
			video.Timestamp = uint32(time.Since(r.StartTime) / time.Millisecond)
		}
		r.VideoInfo.PacketCount++
		r.VideoChan <- video
	}
}
