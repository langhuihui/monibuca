package flv

import (
	"io"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	"os"
	"slices"
	"time"
)

func RecordFlv(ctx *m7s.RecordContext) (err error) {
	var file *os.File
	var filepositions []uint64
	var times []float64
	var offset int64
	var duration int64
	if file, err = os.OpenFile(ctx.FilePath, os.O_CREATE|os.O_RDWR|util.Conditoinal(ctx.Append, os.O_APPEND, os.O_TRUNC), 0666); err != nil {
		return
	}
	suber := ctx.Subscriber
	ar, vr := suber.AudioReader, suber.VideoReader
	hasAudio, hasVideo := ar != nil, vr != nil
	writeMetaTag := func() {
		defer func() {
			err = file.Close()
			if info, err := file.Stat(); err == nil && info.Size() == 0 {
				os.Remove(file.Name())
			}
		}()
		var amf rtmp.AMF
		metaData := rtmp.EcmaArray{
			"MetaDataCreator": "m7s/" + m7s.Version,
			"hasVideo":        hasVideo,
			"hasAudio":        hasAudio,
			"hasMatadata":     true,
			"canSeekToEnd":    true,
			"duration":        float64(duration) / 1000,
			"hasKeyFrames":    len(filepositions) > 0,
			"filesize":        0,
		}
		var flags byte
		if hasAudio {
			ctx := ar.Track.ICodecCtx.GetBase().(pkg.IAudioCodecCtx)
			flags |= (1 << 2)
			metaData["audiocodecid"] = int(rtmp.ParseAudioCodec(ctx.FourCC()))
			metaData["audiosamplerate"] = ctx.GetSampleRate()
			metaData["audiosamplesize"] = ctx.GetSampleSize()
			metaData["stereo"] = ctx.GetChannels() == 2
		}
		if hasVideo {
			ctx := vr.Track.ICodecCtx.GetBase().(pkg.IVideoCodecCtx)
			flags |= 1
			metaData["videocodecid"] = int(rtmp.ParseVideoCodec(ctx.FourCC()))
			metaData["width"] = ctx.Width()
			metaData["height"] = ctx.Height()
			metaData["framerate"] = vr.Track.FPS
			metaData["videodatarate"] = vr.Track.BPS
			metaData["keyframes"] = map[string]any{
				"filepositions": filepositions,
				"times":         times,
			}
			defer func() {
				filepositions = []uint64{0}
				times = []float64{0}
			}()
		}
		amf.Marshals("onMetaData", metaData)
		offset := amf.Len() + 13 + 15
		if keyframesCount := len(filepositions); keyframesCount > 0 {
			metaData["filesize"] = uint64(offset) + filepositions[keyframesCount-1]
			for i := range filepositions {
				filepositions[i] += uint64(offset)
			}
			metaData["keyframes"] = map[string]any{
				"filepositions": filepositions,
				"times":         times,
			}
		}

		if tempFile, err := os.CreateTemp("", "*.flv"); err != nil {
			ctx.Error("create temp file failed", "err", err)
			return
		} else {
			defer func() {
				tempFile.Close()
				os.Remove(tempFile.Name())
				ctx.Info("writeMetaData success")
			}()
			_, err := tempFile.Write([]byte{'F', 'L', 'V', 0x01, flags, 0, 0, 0, 9, 0, 0, 0, 0})
			if err != nil {
				ctx.Error(err.Error())
				return
			}
			amf.Reset()
			marshals := amf.Marshals("onMetaData", metaData)
			WriteFLVTag(tempFile, FLV_TAG_TYPE_SCRIPT, 0, marshals)
			_, err = file.Seek(13, io.SeekStart)
			if err != nil {
				ctx.Error("writeMetaData Seek failed", "err", err)
				return
			}
			_, err = io.Copy(tempFile, file)
			if err != nil {
				ctx.Error("writeMetaData Copy failed", "err", err)
				return
			}
			_, err = tempFile.Seek(0, io.SeekStart)
			_, err = file.Seek(0, io.SeekStart)
			_, err = io.Copy(file, tempFile)
			if err != nil {
				ctx.Error("writeMetaData Copy failed", "err", err)
				return
			}
		}
	}
	if ctx.Append {
		var metaData rtmp.EcmaArray
		metaData, err = ReadMetaData(file)
		keyframes := metaData["keyframes"].(map[string]any)
		filepositions = slices.Collect(func(yield func(uint64) bool) {
			for _, v := range keyframes["filepositions"].([]float64) {
				yield(uint64(v))
			}
		})
		times = keyframes["times"].([]float64)
		if _, err = file.Seek(-4, io.SeekEnd); err != nil {
			ctx.Error("seek file failed", "err", err)
			file.Write(FLVHead)
		} else {
			tmp := make(util.Buffer, 4)
			tmp2 := tmp
			file.Read(tmp)
			tagSize := tmp.ReadUint32()
			tmp = tmp2
			file.Seek(int64(tagSize), io.SeekEnd)
			file.Read(tmp2)
			ts := tmp2.ReadUint24() | (uint32(tmp[3]) << 24)
			ctx.Info("append flv", "last tagSize", tagSize, "last ts", ts)
			if hasVideo {
				vr.StartTs = time.Duration(ts) * time.Millisecond
			}
			if hasAudio {
				ar.StartTs = time.Duration(ts) * time.Millisecond
			}
			file.Seek(0, io.SeekEnd)
		}
	} else {
		file.Write(FLVHead)
	}
	if ctx.Fragment == 0 {
		defer writeMetaTag()
	}
	checkFragment := func(absTime uint32) {
		if ctx.Fragment == 0 {
			return
		}
		if duration = int64(absTime); time.Duration(duration)*time.Millisecond >= ctx.Fragment {
			writeMetaTag()
			offset = 0
			if file, err = os.OpenFile(ctx.FilePath, os.O_CREATE|os.O_RDWR, 0666); err != nil {
				return
			}
			file.Write(FLVHead)
			if vr != nil {
				vr.ResetAbsTime()
				err = WriteFLVTag(file, FLV_TAG_TYPE_VIDEO, 0, vr.Track.SequenceFrame.(*rtmp.RTMPVideo).Buffers...)
			}
		}
	}
	return m7s.PlayBlock(ctx.Subscriber, func(audio *rtmp.RTMPAudio) (err error) {
		if !hasVideo {
			checkFragment(ar.AbsTime)
		}
		return WriteFLVTag(file, FLV_TAG_TYPE_AUDIO, vr.AbsTime, audio.Buffers...)
	}, func(video *rtmp.RTMPVideo) (err error) {
		if vr.Value.IDR {
			filepositions = append(filepositions, uint64(offset))
			times = append(times, float64(vr.AbsTime)/1000)
		}
		return WriteFLVTag(file, FLV_TAG_TYPE_VIDEO, vr.AbsTime, video.Buffers...)
	})
}
