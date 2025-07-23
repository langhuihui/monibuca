package plugin_mp4

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gobwas/ws/wsutil"
	"m7s.live/v5"
	v5 "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/plugin/mp4/pb"
	pkg "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

type MediaContext struct {
	io.Writer
	conn   net.Conn
	wto    time.Duration
	ws     bool
	buffer []byte
}

func (m *MediaContext) Write(p []byte) (n int, err error) {
	if m.ws {
		m.buffer = append(m.buffer, p...)
		return len(p), nil
	}
	if m.conn != nil && m.wto > 0 {
		m.conn.SetWriteDeadline(time.Now().Add(m.wto))
	}
	return m.Writer.Write(p)
}

func (m *MediaContext) Flush() (err error) {
	if m.ws {
		if m.wto > 0 {
			m.conn.SetWriteDeadline(time.Now().Add(m.wto))
		}
		err = wsutil.WriteServerBinary(m.conn, m.buffer)
		m.buffer = m.buffer[:0]
	}
	return
}

type MP4Plugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	BeforeDuration           time.Duration `default:"30s" desc:"事件录像提前时长，不配置则默认30s"`
	AfterDuration            time.Duration `default:"30s" desc:"事件录像结束时长，不配置则默认30s"`
	RecordFileExpireDays     int           `desc:"录像自动删除的天数,0或未设置表示不自动删除"`
	DiskMaxPercent           float64       `default:"90" desc:"硬盘使用百分之上限值，超上限后触发报警，并停止当前所有磁盘写入动作。"`
	AutoOverWriteDiskPercent float64       `default:"0" desc:"自动覆盖功能磁盘占用上限值，超过上限时连续录像自动删除日有录像，事件录像自动删除非重要事件录像，删除规则为删除距离当日最久日期的连续录像或非重要事件录像。"`
	AutoRecovery             bool          `default:"false" desc:"是否自动恢复"`
	ExceptionPostUrl         string        `desc:"第三方异常上报地址"`
	EventRecordFilePath      string        `desc:"事件录像存放地址"`
}

const defaultConfig m7s.DefaultYaml = `publish:
  speed: 1`

// var exceptionChannel = make(chan *Exception)
var _ = m7s.InstallPlugin[MP4Plugin](m7s.PluginMeta{
	DefaultYaml:         defaultConfig,
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
	NewPuller:           pkg.NewPuller,
	NewRecorder:         pkg.NewRecorder,
	NewPullProxy:        m7s.NewHTTPPullPorxy,
})

func (p *MP4Plugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/download/{streamPath...}":           p.download,
		"/extract/compressed/{streamPath...}": p.extractCompressedVideoHandel,
		"/extract/gop/{streamPath...}":        p.extractGopVideoHandel,
		"/snap/{streamPath...}":               p.snapHandel,
	}
}

func (p *MP4Plugin) OnInit() (err error) {
	if p.DB != nil {
		err = p.DB.AutoMigrate(&Exception{})
		if err != nil {
			return
		}
		if p.AutoOverWriteDiskPercent > 0 {
			var deleteRecordTask DeleteRecordTask
			deleteRecordTask.DB = p.DB
			deleteRecordTask.DiskMaxPercent = p.DiskMaxPercent
			deleteRecordTask.AutoOverWriteDiskPercent = p.AutoOverWriteDiskPercent
			deleteRecordTask.RecordFileExpireDays = p.RecordFileExpireDays
			deleteRecordTask.plugin = p
			p.AddTask(&deleteRecordTask)
		}
		if p.AutoRecovery {
			var recoveryTask RecordRecoveryTask
			recoveryTask.DB = p.DB
			recoveryTask.plugin = p
			p.AddTask(&recoveryTask)
		}
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
	ctx.wto = p.GetCommonConf().WriteTimeout
	if ctx.conn == nil {
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		if hijacker, ok := w.(http.Hijacker); ok && ctx.wto > 0 {
			ctx.conn, _, _ = hijacker.Hijack()
			ctx.conn.SetWriteDeadline(time.Now().Add(ctx.wto))
			ctx.Writer = ctx.conn
		} else {
			ctx.Writer = w
			w.(http.Flusher).Flush()
		}
	} else {
		ctx.ws = true
		ctx.Writer = ctx.conn
	}

	muxer := pkg.NewMuxer(pkg.FLAG_FRAGMENT)
	err = muxer.WriteInitSegment(&ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var offsetAudio, offsetVideo = 1, 5
	var audio, video *pkg.Track
	var nextFragmentId uint32
	if sub.Publisher.HasVideoTrack() && sub.SubVideo {
		v := sub.Publisher.VideoTrack.AVTrack
		if err = v.WaitReady(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var codecID box.MP4_CODEC_TYPE
		switch v.ICodecCtx.FourCC() {
		case codec.FourCC_H264:
			codecID = box.MP4_CODEC_H264
		case codec.FourCC_H265:
			codecID = box.MP4_CODEC_H265
		}
		video = muxer.AddTrack(codecID)
		video.Timescale = 1000
		video.Samplelist = []box.Sample{
			{
				Offset:    0,
				Data:      nil,
				Size:      0,
				Timestamp: 0,
				Duration:  0,
				KeyFrame:  true,
			},
		}
		switch v.ICodecCtx.FourCC() {
		case codec.FourCC_H264:
			h264Ctx := v.ICodecCtx.GetBase().(*codec.H264Ctx)
			video.ExtraData = h264Ctx.Record
			video.Width = uint32(h264Ctx.Width())
			video.Height = uint32(h264Ctx.Height())
		case codec.FourCC_H265:
			h265Ctx := v.ICodecCtx.GetBase().(*codec.H265Ctx)
			video.ExtraData = h265Ctx.Record
			video.Width = uint32(h265Ctx.Width())
			video.Height = uint32(h265Ctx.Height())
		}
	}

	if sub.Publisher.HasAudioTrack() && sub.SubAudio {
		a := sub.Publisher.AudioTrack.AVTrack
		if err = a.WaitReady(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var codecID box.MP4_CODEC_TYPE
		switch a.ICodecCtx.FourCC() {
		case codec.FourCC_MP4A:
			codecID = box.MP4_CODEC_AAC
		case codec.FourCC_ALAW:
			codecID = box.MP4_CODEC_G711A
		case codec.FourCC_ULAW:
			codecID = box.MP4_CODEC_G711U
		case codec.FourCC_OPUS:
			codecID = box.MP4_CODEC_OPUS
		}
		audio = muxer.AddTrack(codecID)
		audio.Timescale = 1000
		audioCtx := a.ICodecCtx.(v5.IAudioCodecCtx)
		audio.SampleRate = uint32(audioCtx.GetSampleRate())
		audio.ChannelCount = uint8(audioCtx.GetChannels())
		audio.SampleSize = uint16(audioCtx.GetSampleSize())
		audio.Samplelist = []box.Sample{
			{
				Offset:    0,
				Data:      nil,
				Size:      0,
				Timestamp: 0,
				Duration:  0,
				KeyFrame:  true,
			},
		}
		switch a.ICodecCtx.FourCC() {
		case codec.FourCC_MP4A:
			offsetAudio = 2
			audio.ExtraData = a.ICodecCtx.GetBase().(*codec.AACCtx).ConfigBytes
		default:
			offsetAudio = 1
		}
	}
	err = muxer.WriteMoov(&ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ctx.ws {
		ctx.Flush()
	}
	m7s.PlayBlock(sub, func(frame *rtmp.RTMPAudio) (err error) {
		bs := frame.Memory.ToBytes()
		if offsetAudio == 2 && bs[1] == 0 {
			return nil
		}
		if audio.Samplelist[0].Data != nil {
			audio.Samplelist[0].Duration = sub.AudioReader.AbsTime - audio.Samplelist[0].Timestamp
			nextFragmentId++
			// Create moof box for this track
			moof := audio.MakeMoof(nextFragmentId)
			// Create mdat box for this track
			mdat := box.CreateDataBox(box.TypeMDAT, audio.Samplelist[0].Data)
			box.WriteTo(&ctx, moof, mdat)
			if ctx.ws {
				err = ctx.Flush()
			}
		}
		audio.Samplelist[0].Timestamp = sub.AudioReader.AbsTime
		audio.Samplelist[0].Data = bs[offsetAudio:]
		audio.Samplelist[0].Size = len(audio.Samplelist[0].Data)
		return
	}, func(frame *rtmp.RTMPVideo) (err error) {
		bs := frame.Memory.ToBytes()
		if ctx, ok := sub.VideoReader.Track.ICodecCtx.(*rtmp.H265Ctx); ok && ctx.Enhanced {
			switch bs[0] & 0b1111 {
			case rtmp.PacketTypeCodedFrames:
				offsetVideo = 8
			case rtmp.PacketTypeSequenceStart:
				return nil
			}
		} else {
			if bs[1] == 0 {
				return nil
			}
			offsetVideo = 5
		}
		if video.Samplelist[0].Data != nil {
			video.Samplelist[0].Duration = sub.VideoReader.AbsTime - video.Samplelist[0].Timestamp
			nextFragmentId++
			// Create moof box for this track
			moof := video.MakeMoof(nextFragmentId)
			// Create mdat box for this track
			mdat := box.CreateDataBox(box.TypeMDAT, video.Samplelist[0].Data)
			box.WriteTo(&ctx, moof, mdat)
			if ctx.ws {
				err = ctx.Flush()
			}
		}
		video.Samplelist[0].Data = bs[offsetVideo:]
		video.Samplelist[0].Size = len(bs) - offsetVideo
		video.Samplelist[0].Timestamp = sub.VideoReader.AbsTime
		video.Samplelist[0].CTS = frame.CTS
		video.Samplelist[0].KeyFrame = sub.VideoReader.Value.IDR
		return
	})
}
