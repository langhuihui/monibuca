package plugin_mp4

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/mp4/pb"
	mp4pkg "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

type MP4Plugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	BeforeDuration       time.Duration `default:"30s" desc:"事件录像提前时长，不配置则默认30s"`
	AfterDuration        time.Duration `default:"30s" desc:"事件录像结束时长，不配置则默认30s"`
	RecordFileExpireDays int           `desc:"录像自动删除的天数,0或未设置表示不自动删除"`
	DiskMaxPercent       float64       `default:"90" desc:"硬盘使用百分之上限值，超上限后触发报警，并停止当前所有磁盘写入动作。"`
	OverwritePercent     float64       `default:"0" desc:"全局磁盘使用率阈值，当 storage.local 的 overwritepercent 为 0 时使用此值作为全局兼底配置。超过阈值时自动迁移或删除最旧文件。"`
	AutoRecovery         bool          `default:"false" desc:"是否自动恢复"`
	ExceptionPostUrl     string        `desc:"第三方异常上报地址"`
	EventRecordFilePath  string        `desc:"事件录像存放地址"`
}

const defaultConfig m7s.DefaultYaml = `publish:
  speed: 1`

// var exceptionChannel = make(chan *Exception)
var _ = m7s.InstallPlugin[MP4Plugin](m7s.PluginMeta{
	DefaultYaml:         defaultConfig,
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
	NewPuller:           mp4pkg.NewPuller,
	NewRecorder:         mp4pkg.NewRecorder,
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

func (p *MP4Plugin) Start() (err error) {
	if p.DB != nil {
		err = p.DB.AutoMigrate(&Exception{})
		if err != nil {
			return
		}
		err = p.DB.AutoMigrate(&mp4pkg.TagModel{})
		if err != nil {
			return
		}
		if p.OverwritePercent > 0 {
			var storageTask StorageManagementTask
			storageTask.DB = p.DB
			storageTask.DiskMaxPercent = p.DiskMaxPercent
			storageTask.OverwritePercent = p.OverwritePercent
			storageTask.RecordFileExpireDays = p.RecordFileExpireDays
			storageTask.plugin = p
			p.AddTask(&storageTask)
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
	redirectPath := strings.TrimPrefix(r.URL.Path, "/")
	if p.Server != nil && p.Server.RedirectIfNeeded(w, r, "mp4", redirectPath) {
		p.Debug("redirect issued", "protocol", "http", "path", redirectPath)
		return
	}
	streamPath := strings.TrimSuffix(redirectPath, ".mp4")
	if r.URL.RawQuery != "" {
		streamPath += "?" + r.URL.RawQuery
	}
	sub, err := p.Subscribe(r.Context(), streamPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sub.RemoteAddr = r.RemoteAddr
	var ctx util.HTTP_WS_Writer
	ctx.Conn, err = sub.CheckWebSocket(w, r)
	if err != nil {
		return
	}
	ctx.WriteTimeout = p.GetCommonConf().WriteTimeout
	ctx.ContentType = "video/mp4"
	ctx.ServeHTTP(w, r)

	muxer := mp4pkg.NewMuxer(mp4pkg.FLAG_FRAGMENT)
	err = muxer.WriteInitSegment(&ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var audio, video *mp4pkg.Track
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
				KeyFrame: true,
			},
		}
		video.ICodecCtx = v.ICodecCtx
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
		audio.ICodecCtx = a.ICodecCtx
		audio.Samplelist = []box.Sample{
			{
				KeyFrame: true,
			},
		}
	}
	err = muxer.WriteMoov(&ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx.Flush()
	m7s.PlayBlock(sub, func(frame *mp4pkg.AudioFrame) (err error) {
		if audio.Samplelist[0].Buffers != nil {
			audio.Samplelist[0].Duration = sub.AudioReader.AbsTime - audio.Samplelist[0].Timestamp
			nextFragmentId++
			// Create moof box for this track
			moof := audio.MakeMoof(nextFragmentId)
			// Create mdat box for this track
			mdat := box.CreateMemoryBox(box.TypeMDAT, audio.Samplelist[0].Memory)
			box.WriteTo(&ctx, moof, mdat)
			err = ctx.Flush()
		}
		audio.Samplelist[0].Timestamp = sub.AudioReader.AbsTime
		audio.Samplelist[0].Memory = frame.Memory
		return
	}, func(frame *mp4pkg.VideoFrame) (err error) {
		if video.Samplelist[0].Buffers != nil {
			video.Samplelist[0].Duration = sub.VideoReader.AbsTime - video.Samplelist[0].Timestamp
			nextFragmentId++
			// Create moof box for this track
			moof := video.MakeMoof(nextFragmentId)
			// Create mdat box for this track
			mdat := box.CreateMemoryBox(box.TypeMDAT, video.Samplelist[0].Memory)
			box.WriteTo(&ctx, moof, mdat)
			err = ctx.Flush()
		}
		video.Samplelist[0].Memory = frame.Memory
		video.Samplelist[0].Timestamp = sub.VideoReader.AbsTime
		video.Samplelist[0].CTS = uint32(frame.CTS / time.Millisecond)
		video.Samplelist[0].KeyFrame = sub.VideoReader.Value.IDR
		return
	})
}
