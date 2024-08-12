package plugin_mp4

import (
	"io"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	"m7s.live/m7s/v5"
	v5 "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
	pkg "m7s.live/m7s/v5/plugin/mp4/pkg"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
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
	m7s.Plugin
}

const defaultConfig m7s.DefaultYaml = `publish:
  speed: 1`

func (p *MP4Plugin) OnInit() error {
	for streamPath, url := range p.GetCommonConf().PullOnStart {
		p.Pull(streamPath, url)
	}
	return nil
}

var _ = m7s.InstallPlugin[MP4Plugin](defaultConfig, pkg.PullMP4, pkg.RecordMP4)

func (p *MP4Plugin) GetPullableList() []string {
	return slices.Collect(maps.Keys(p.GetCommonConf().PullOnSub))
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
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Type", "video/mp4")
	w.WriteHeader(http.StatusOK)
	initSegment := mp4.CreateEmptyInit()
	initSegment.Moov.Mvhd.NextTrackID = 1
	var ctx MediaContext
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
	if hijacker, ok := w.(http.Hijacker); ok && ctx.wto > 0 {
		ctx.conn, _, _ = hijacker.Hijack()
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
		b0 := bs[0]
		idr := b0&0b0111_0000>>4 == 1
		if b0&0b1000_0000 == 0 {
			offsetVideo = 5
			if bs[1] == 0 {
				return nil
			}
		} else {
			switch packetType := b0 & 0b1111; packetType {
			case rtmp.PacketTypeSequenceStart:
				return nil
			case rtmp.PacketTypeCodedFrames:
				offsetVideo = 8
			case rtmp.PacketTypeCodedFramesX:
				offsetVideo = 5
			}
		}
		ctx.video.Push(&ctx, video.Timestamp, durVideo, bs[offsetVideo:], util.Conditoinal(idr, mp4.SyncSampleFlags, mp4.NonSyncSampleFlags))
		lastVTime = video.Timestamp
		return nil
	})
}
