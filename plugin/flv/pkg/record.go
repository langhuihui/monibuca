package flv

import (
	"io"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	"os"
)

type Recorder struct {
	*m7s.Subscriber
	filepositions []uint64
	times         []float64
	Offset        int64
	duration      int64
}

func (r *Recorder) Record(recorder *m7s.Recorder) (err error) {
	return
}

func (r *Recorder) Close() {

}

func (r *Recorder) writeMetaData(file util.ReadWriteSeekCloser, duration int64) {
	defer file.Close()
	at, vt := r.AudioReader, r.VideoReader
	hasAudio, hasVideo := at != nil, vt != nil
	var amf rtmp.AMF
	metaData := rtmp.EcmaArray{
		"MetaDataCreator": "m7s/" + m7s.Version,
		"hasVideo":        hasVideo,
		"hasAudio":        hasAudio,
		"hasMatadata":     true,
		"canSeekToEnd":    true,
		"duration":        float64(duration) / 1000,
		"hasKeyFrames":    len(r.filepositions) > 0,
		"filesize":        0,
	}
	var flags byte
	if hasAudio {
		ctx := at.Track.ICodecCtx.GetBase().(pkg.IAudioCodecCtx)
		flags |= (1 << 2)
		metaData["audiocodecid"] = int(rtmp.ParseAudioCodec(ctx.FourCC()))
		metaData["audiosamplerate"] = ctx.GetSampleRate()
		metaData["audiosamplesize"] = ctx.GetSampleSize()
		metaData["stereo"] = ctx.GetChannels() == 2
	}
	if hasVideo {
		ctx := vt.Track.ICodecCtx.GetBase().(pkg.IVideoCodecCtx)
		flags |= 1
		metaData["videocodecid"] = int(rtmp.ParseVideoCodec(ctx.FourCC()))
		metaData["width"] = ctx.Width()
		metaData["height"] = ctx.Height()
		metaData["framerate"] = vt.Track.FPS
		metaData["videodatarate"] = vt.Track.BPS
		metaData["keyframes"] = map[string]any{
			"filepositions": r.filepositions,
			"times":         r.times,
		}
		defer func() {
			r.filepositions = []uint64{0}
			r.times = []float64{0}
		}()
	}
	amf.Marshals("onMetaData", metaData)
	offset := amf.Len() + 13 + 15
	if keyframesCount := len(r.filepositions); keyframesCount > 0 {
		metaData["filesize"] = uint64(offset) + r.filepositions[keyframesCount-1]
		for i := range r.filepositions {
			r.filepositions[i] += uint64(offset)
		}
		metaData["keyframes"] = map[string]any{
			"filepositions": r.filepositions,
			"times":         r.times,
		}
	}

	if tempFile, err := os.CreateTemp("", "*.flv"); err != nil {
		r.Error("create temp file failed", "err", err)
		return
	} else {
		defer func() {
			tempFile.Close()
			os.Remove(tempFile.Name())
			r.Info("writeMetaData success")
		}()
		_, err := tempFile.Write([]byte{'F', 'L', 'V', 0x01, flags, 0, 0, 0, 9, 0, 0, 0, 0})
		if err != nil {
			r.Error(err.Error())
			return
		}
		amf.Reset()
		marshals := amf.Marshals("onMetaData", metaData)
		WriteFLVTag(tempFile, FLV_TAG_TYPE_SCRIPT, 0, marshals)
		_, err = file.Seek(13, io.SeekStart)
		if err != nil {
			r.Error("writeMetaData Seek failed", "err", err)
			return
		}
		_, err = io.Copy(tempFile, file)
		if err != nil {
			r.Error("writeMetaData Copy failed", "err", err)
			return
		}
		_, err = tempFile.Seek(0, io.SeekStart)
		_, err = file.Seek(0, io.SeekStart)
		_, err = io.Copy(file, tempFile)
		if err != nil {
			r.Error("writeMetaData Copy failed", "err", err)
			return
		}
	}
}
