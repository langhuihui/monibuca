package plugin_mp4

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	"m7s.live/v5"
	v5 "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/mp4/pb"
	pkg "m7s.live/v5/plugin/mp4/pkg"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

type MediaContext struct {
	io.Writer
	conn         net.Conn
	wto          time.Duration
	seqNumber    uint32
	audio, video TrackContext
}

func (m *MediaContext) Write(p []byte) (n int, err error) {
	if m.conn != nil {
		m.conn.SetWriteDeadline(time.Now().Add(m.wto))
	}
	return m.Writer.Write(p)
}

type TrackContext struct {
	TrackId  uint32
	fragment *mp4.Fragment
	ts       uint32 // 每个小片段起始时间戳
	abs      uint32 // 绝对起始时间戳
	absSet   bool   // 是否设置过abs
}

func (m *TrackContext) Push(ctx *MediaContext, dt uint32, dur uint32, data []byte, flags uint32) {
	if !m.absSet {
		m.abs = dt
		m.absSet = true
	}
	dt -= m.abs
	if m.fragment != nil && dt-m.ts > 1000 {
		m.fragment.Encode(ctx)
		m.fragment = nil
	}
	if m.fragment == nil {
		ctx.seqNumber++
		m.fragment, _ = mp4.CreateFragment(ctx.seqNumber, m.TrackId)
		m.ts = dt
	}
	m.fragment.AddFullSample(mp4.FullSample{
		Data:       data,
		DecodeTime: uint64(dt),
		Sample: mp4.Sample{
			Flags: flags,
			Dur:   dur,
			Size:  uint32(len(data)),
		},
	})
}

type MP4Plugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	BeforeDuration           time.Duration `default:"30s" desc:"事件录像提前时长，不配置则默认30s"`
	AfterDuration            time.Duration `default:"30s" desc:"事件录像结束时长，不配置则默认30s"`
	RecordFileExpireDays     int           `desc:"录像自动删除的天数,0或未设置表示不自动删除"`
	DiskMaxPercent           float64       `default:"90" desc:"硬盘使用百分之上限值，超上限后触发报警，并停止当前所有磁盘写入动作。"`
	AutoOverWriteDiskPercent float64       `default:"80" desc:"自动覆盖功能磁盘占用上限值，超过上限时连续录像自动删除日有录像，事件录像自动删除非重要事件录像，删除规则为删除距离当日最久日期的连续录像或非重要事件录像。"`
	ExceptionPostUrl         string        `desc:"第三方异常上报地址"`
	EventRecordFilePath      string        `desc:"事件录像存放地址"`
}

const defaultConfig m7s.DefaultYaml = `publish:
  speed: 1`

// var exceptionChannel = make(chan *Exception)
var _ = m7s.InstallPlugin[MP4Plugin](defaultConfig, &pb.Api_ServiceDesc, pb.RegisterApiHandler, pkg.NewPuller, pkg.NewRecorder)

func (p *MP4Plugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/download/{streamPath...}": p.download,
	}
}
func (p *MP4Plugin) OnInit() (err error) {
	if p.DB != nil {
		err = p.DB.AutoMigrate(&Exception{})
		var deleteRecordTask DeleteRecordTask
		deleteRecordTask.DB = p.DB
		deleteRecordTask.DiskMaxPercent = p.DiskMaxPercent
		deleteRecordTask.AutoOverWriteDiskPercent = p.AutoOverWriteDiskPercent
		deleteRecordTask.RecordFileExpireDays = p.RecordFileExpireDays
		p.AddTask(&deleteRecordTask)
	}
	// go func() { //处理所有异常，录像中断异常、录像读取异常、录像导出文件中断、磁盘容量低于阈值异常、磁盘异常
	// 	for exception := range exceptionChannel {
	// 		p.SendToThirdPartyAPI(exception)
	// 	}
	// }()
	_, port, _ := strings.Cut(p.GetCommonConf().HTTP.ListenAddr, ":")
	if port == "80" {
		p.PlayAddr = append(p.PlayAddr, "http://{hostName}/mp4/{streamPath}.mp4")
	} else if port != "" {
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("http://{hostName}:%s/mp4/{streamPath}.mp4", port))
	}
	_, port, _ = strings.Cut(p.GetCommonConf().HTTP.ListenAddrTLS, ":")
	if port == "443" {
		p.PlayAddr = append(p.PlayAddr, "https://{hostName}/mp4/{streamPath}.mp4")
	} else if port != "" {
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("https://{hostName}:%s/mp4/{streamPath}.mp4", port))
	}
	return
}
func (p *MP4Plugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	streamPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".mp4")
	if r.URL.RawQuery != "" {
		streamPath += "?" + r.URL.RawQuery
	}
	sub, err := p.Subscribe(r.Context(), streamPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sub.RemoteAddr = r.RemoteAddr
	var ctx MediaContext
	ctx.conn, err = sub.CheckWebSocket(w, r)
	if err != nil {
		return
	}
	wto := p.GetCommonConf().WriteTimeout
	if ctx.conn == nil {
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		if hijacker, ok := w.(http.Hijacker); ok && wto > 0 {
			ctx.conn, _, _ = hijacker.Hijack()
			ctx.conn.SetWriteDeadline(time.Now().Add(wto))
		}
	}

	initSegment := mp4.CreateEmptyInit()
	initSegment.Moov.Mvhd.NextTrackID = 1

	ctx.wto = p.GetCommonConf().WriteTimeout
	var ftyp *mp4.FtypBox
	var offsetAudio, offsetVideo = 1, 5
	var durAudio, durVideo uint32 = 40, 40
	if sub.Publisher.HasVideoTrack() {
		v := sub.Publisher.VideoTrack.AVTrack
		if err = v.WaitReady(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		moov := initSegment.Moov
		trackID := moov.Mvhd.NextTrackID
		moov.Mvhd.NextTrackID++
		newTrak := mp4.CreateEmptyTrak(trackID, 1000, "video", "chi")
		moov.AddChild(newTrak)
		moov.Mvex.AddChild(mp4.CreateTrex(trackID))
		ctx.video.TrackId = trackID
		ftyp = mp4.NewFtyp("isom", 0x200, []string{
			"isom", "iso2", v.ICodecCtx.FourCC().String(), "mp41",
		})
		switch v.ICodecCtx.FourCC() {
		case codec.FourCC_H264:
			h264Ctx := v.ICodecCtx.GetBase().(*codec.H264Ctx)
			durVideo = uint32(h264Ctx.PacketDuration(nil) / time.Millisecond)
			newTrak.SetAVCDescriptor("avc1", h264Ctx.RecordInfo.SPS, h264Ctx.RecordInfo.PPS, true)
		case codec.FourCC_H265:
			h265Ctx := v.ICodecCtx.GetBase().(*codec.H265Ctx)
			durVideo = uint32(h265Ctx.PacketDuration(nil) / time.Millisecond)
			newTrak.SetHEVCDescriptor("hvc1", h265Ctx.RecordInfo.VPS, h265Ctx.RecordInfo.SPS, h265Ctx.RecordInfo.PPS, nil, true)
		case codec.FourCC_AV1:
			//av1Ctx := v.ICodecCtx.GetBase().(*codec.AV1Ctx)
			//durVideo = uint32(av1Ctx.PacketDuration(nil) / time.Millisecond)
		}
	}
	if sub.Publisher.HasAudioTrack() {
		a := sub.Publisher.AudioTrack.AVTrack
		if err = a.WaitReady(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		moov := initSegment.Moov
		trackID := moov.Mvhd.NextTrackID
		moov.Mvhd.NextTrackID++
		newTrak := mp4.CreateEmptyTrak(trackID, 1000, "audio", "chi")
		moov.AddChild(newTrak)
		moov.Mvex.AddChild(mp4.CreateTrex(trackID))
		ctx.audio.TrackId = trackID
		audioCtx := a.ICodecCtx.(v5.IAudioCodecCtx)
		switch a.ICodecCtx.FourCC() {
		case codec.FourCC_MP4A:
			offsetAudio = 2
			aacCtx := a.ICodecCtx.GetBase().(*codec.AACCtx)
			newTrak.SetAACDescriptor(byte(aacCtx.Config.ObjectType), aacCtx.Config.SampleRate)
		case codec.FourCC_ALAW:
			stsd := newTrak.Mdia.Minf.Stbl.Stsd
			pcma := mp4.CreateAudioSampleEntryBox("pcma",
				uint16(audioCtx.GetChannels()),
				uint16(audioCtx.GetSampleSize()), uint16(audioCtx.GetSampleRate()), nil)
			stsd.AddChild(pcma)
		case codec.FourCC_ULAW:
			stsd := newTrak.Mdia.Minf.Stbl.Stsd
			pcmu := mp4.CreateAudioSampleEntryBox("pcmu",
				uint16(audioCtx.GetChannels()),
				uint16(audioCtx.GetSampleSize()), uint16(audioCtx.GetSampleRate()), nil)
			stsd.AddChild(pcmu)
		}
	}
	if ctx.conn != nil {
		ctx.Writer = ctx.conn
	} else {
		ctx.Writer = w
		w.(http.Flusher).Flush()
	}
	ftyp.Encode(&ctx)
	initSegment.Moov.Encode(&ctx)
	var lastATime, lastVTime uint32
	m7s.PlayBlock(sub, func(audio *rtmp.RTMPAudio) error {
		bs := audio.Memory.ToBytes()
		if offsetAudio == 2 && bs[1] == 0 {
			return nil
		}
		if lastATime > 0 {
			durAudio = audio.Timestamp - lastATime
		}
		ctx.audio.Push(&ctx, audio.Timestamp, durAudio, bs[offsetAudio:], mp4.SyncSampleFlags)
		lastATime = audio.Timestamp
		return nil
	}, func(video *rtmp.RTMPVideo) error {
		if lastVTime > 0 {
			durVideo = video.Timestamp - lastVTime
		}
		bs := video.Memory.ToBytes()
		if ctx, ok := sub.VideoReader.Track.ICodecCtx.(*rtmp.H265Ctx); ok && ctx.Enhanced && bs[0]&0b1111 == rtmp.PacketTypeCodedFrames {
			offsetVideo = 8
		} else {
			offsetVideo = 5
		}
		ctx.video.Push(&ctx, video.Timestamp, durVideo, bs[offsetVideo:], util.Conditional(sub.VideoReader.Value.IDR, mp4.SyncSampleFlags, mp4.NonSyncSampleFlags))
		lastVTime = video.Timestamp
		return nil
	})
}
