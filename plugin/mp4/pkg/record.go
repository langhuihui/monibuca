package mp4

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	task "github.com/langhuihui/gotask"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/storage"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/mp4/pkg/box"

	"github.com/langhuihui/gomem"
)

type WriteTrailerQueueTask struct {
	task.Work
}

var writeTrailerQueueTask WriteTrailerQueueTask

type writeTrailerTask struct {
	task.Task
	muxer    *Muxer
	file     storage.File
	filePath string
}

func (task *writeTrailerTask) Start() (err error) {
	err = task.muxer.WriteTrailer(task.file)
	if err != nil {
		task.Error("write trailer", "err", err)
		if task.file != nil {
			if errClose := task.file.Close(); errClose != nil {
				return errClose
			}
		}
	}
	return
}

const BeforeMdatData = 16 // free box + mdat box header or big mdat box header
// 将 moov 从末尾移动到前方
// 将 ftyp + free(optional) + moov + mdat 写入临时文件, 然后替换原文件
func (t *writeTrailerTask) Run() (err error) {
	t.Info("write trailer")
	var temp *os.File
	temp, err = os.CreateTemp("", "*.mp4")
	if err != nil {
		t.Error("create temp file", "err", err)
		return
	}

	defer os.Remove(temp.Name())

	_, err = t.file.Seek(0, io.SeekStart)
	if err != nil {
		t.Error("seek file", "err", err)
		return
	}
	// 复制 mdat box之前的内容
	_, err = io.CopyN(temp, t.file, int64(t.muxer.mdatOffset)-BeforeMdatData)
	if err != nil {
		t.Error("copy file", "err", err)
		return
	}
	for _, track := range t.muxer.Tracks {
		for i := range len(track.Samplelist) {
			track.Samplelist[i].Offset += int64(t.muxer.moov.Size())
		}
	}
	err = t.muxer.WriteMoov(temp)
	if err != nil {
		t.Error("rewrite with moov", "err", err)
		return
	}
	// 复制 mdat box
	_, err = io.CopyN(temp, t.file, int64(t.muxer.mdatSize)+BeforeMdatData)

	if err != nil {
		if err == pkg.ErrSkip {
			return task.ErrTaskComplete
		}
		t.Error("rewrite with moov", "err", err)
		return
	}
	if _, err = t.file.Seek(0, io.SeekStart); err != nil {
		t.Error("seek file", "err", err)
		return
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		t.Error("seek temp file", "err", err)
		return
	}
	if _, err = io.Copy(t.file, temp); err != nil {
		t.Error("copy file", "err", err)
		return
	}
	if err = t.file.Close(); err != nil {
		t.Error("close file", "err", err)
		return
	}
	if err = temp.Close(); err != nil {
		t.Error("close temp file", "err", err)
	}

	return
}

func init() {
	m7s.Servers.AddTask(&writeTrailerQueueTask)
}

func NewRecorder(conf config.Record) m7s.IRecorder {
	return &Recorder{}
}

type bufferedSample struct {
	isAudio  bool
	codecCtx codec.ICodecCtx
	sample   box.Sample
}

type Recorder struct {
	m7s.DefaultRecorder
	muxer           *Muxer
	file            storage.File
	firstVideoFrame bool // 标记是否是第一个视频帧
	creating        bool
	createDone      chan error
	sampleBuffer    []bufferedSample
}

func (r *Recorder) writeTailer(end time.Time) {
	r.WriteTail(end, &writeTrailerQueueTask)
	writeTrailerQueueTask.AddTask(&writeTrailerTask{
		muxer:    r.muxer,
		file:     r.file,
		filePath: r.Event.FilePath,
	}, r.Logger)
}

var CustomFileName = func(job *m7s.RecordJob) string {
	now := time.Now()
	return filepath.Join(job.RecConf.FilePath, fmt.Sprintf("%s_%09d.mp4", time.Now().Local().Format("2006-01-02-15-04-05"), now.Nanosecond()))
}

func (r *Recorder) createStream(start time.Time) (err error) {
	if r.RecordJob.RecConf.Type == "" {
		r.RecordJob.RecConf.Type = "mp4"
	}
	t0 := time.Now()
	err = r.CreateStream(start, CustomFileName)
	r.Info("createStream step1 CreateStream", "elapsed", time.Since(t0))
	if err != nil {
		return
	}

	// 注意: 不要在这里关闭旧文件,因为它已经被传递给 writeTrailerTask
	// writeTrailerTask 会负责关闭旧文件
	// 直接创建新文件并覆盖 r.file

	// 获取存储实例
	st := r.RecordJob.GetStorage()

	if st == nil {
		return fmt.Errorf("global storage is nil")
	}
	t1 := time.Now()
	// 使用存储抽象层
	r.file, err = st.CreateFile(context.Background(), r.Event.FilePath)
	r.Info("createStream step2 CreateFile", "elapsed", time.Since(t1), "path", r.Event.FilePath)
	if err != nil {
		return
	}

	if r.Event.Type == "fmp4" {
		r.muxer = NewMuxerWithStreamPath(FLAG_FRAGMENT, r.Event.StreamPath)
	} else {
		r.muxer = NewMuxerWithStreamPath(0, r.Event.StreamPath)
	}
	t2 := time.Now()
	err = r.muxer.WriteInitSegment(r.file)
	r.Info("createStream step3 WriteInitSegment", "elapsed", time.Since(t2))
	r.Info("createStream total", "elapsed", time.Since(t0))
	r.SetDescription("startTime", start.Format("2006-01-02 15:04:05"))
	return
}

func (r *Recorder) Dispose() {
	if r.creating {
		// 异步分片 createStream 正在进行:OLD 文件已在 checkFragment 的 writeTailer 中移交给 writeTrailerTask。
		// 等待 goroutine 结束,避免它在 retry Run() 启动后仍然修改 r.muxer/r.file 造成竞争。
		if r.createDone != nil {
			<-r.createDone
		}
		r.creating = false
		// goroutine 若成功,r.muxer/r.file 已指向新文件(仅含 init segment)。关闭并丢弃它。
		if r.muxer != nil && r.file != nil {
			r.file.Close()
		}
		r.muxer = nil
		r.file = nil
		return
	}
	if r.muxer != nil {
		r.writeTailer(time.Now())
		// 关键修复:将 muxer 和 file 置 nil,切断重试 Run() 对旧 muxer/file 的访问。
		// 文件的关闭由 writeTrailerTask.Run() 负责。若不置 nil,重试 Run() 会向
		// writeTrailerTask 正在处理的同一 muxer 写入新数据,导致 mdatSize 不匹配→EOF。
		r.muxer = nil
		r.file = nil
	} else {
		if r.file != nil {
			r.file.Close()
			r.file = nil
		}
	}
}

func (r *Recorder) Run() (err error) {
	// 重试时清理上一次运行的缓存状态。
	r.sampleBuffer = r.sampleBuffer[:0]
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	var audioTrack, videoTrack *Track
	var at, vt *pkg.AVTrack
	checkEventRecordStop := func(absTime uint32) (err error) {
		if absTime >= recordJob.Event.AfterDuration+recordJob.Event.BeforeDuration {
			r.RecordJob.Stop(task.ErrStopByUser)
		}
		return
	}

	checkFragment := func(reader *pkg.AVRingReader) (err error) {
		if r.creating {
			return
		}
		if duration := int64(reader.AbsTime); time.Duration(duration)*time.Millisecond >= recordJob.RecConf.Fragment {
			r.writeTailer(reader.Value.WriteTime)
			r.Info("check fragment start async", "absTime", reader.AbsTime, "seq", reader.Value.Sequence)
			startTime := reader.Value.WriteTime
			r.creating = true
			r.createDone = make(chan error, 1)
			r.sampleBuffer = r.sampleBuffer[:0]
			go func() {
				createErr := r.createStream(startTime)
				r.Info("check fragment end async", "err", createErr)
				r.createDone <- createErr
			}()
			at, vt = nil, nil
			if vr := sub.VideoReader; vr != nil {
				vr.ResetAbsTime()
			}
			if ar := sub.AudioReader; ar != nil {
				ar.ResetAbsTime()
			}
		}
		return
	}

	// flushBuffer 将 createStream 异步执行期间缓存的帧写入新文件
	flushBuffer := func() error {
		for _, bs := range r.sampleBuffer {
			if bs.isAudio {
				if at == nil {
					at = sub.AudioReader.Track
					switch bs.codecCtx.GetBase().(type) {
					case *codec.AACCtx:
						track := r.muxer.AddTrack(box.MP4_CODEC_AAC)
						audioTrack = track
						track.ICodecCtx = bs.codecCtx
					case *codec.PCMACtx:
						track := r.muxer.AddTrack(box.MP4_CODEC_G711A)
						audioTrack = track
						track.ICodecCtx = bs.codecCtx
					case *codec.PCMUCtx:
						track := r.muxer.AddTrack(box.MP4_CODEC_G711U)
						audioTrack = track
						track.ICodecCtx = bs.codecCtx
					}
				}
				if err := r.muxer.WriteSample(r.file, audioTrack, bs.sample); err != nil {
					return err
				}
			} else {
				if vt == nil {
					vt = sub.VideoReader.Track
					switch bs.codecCtx.GetBase().(type) {
					case *codec.H264Ctx:
						track := r.muxer.AddTrack(box.MP4_CODEC_H264)
						videoTrack = track
						track.ICodecCtx = bs.codecCtx
					case *codec.H265Ctx:
						track := r.muxer.AddTrack(box.MP4_CODEC_H265)
						videoTrack = track
						track.ICodecCtx = bs.codecCtx
					}
				}
				if err := r.muxer.WriteSample(r.file, videoTrack, bs.sample); err != nil {
					return err
				}
			}
		}
		r.sampleBuffer = r.sampleBuffer[:0]
		return nil
	}

	return m7s.PlayBlock(sub, func(audio *AudioFrame) error {
		// 用 r.muxer == nil 替代 r.Event.StartTime.IsZero():
		// Dispose() 已将 r.muxer 置 nil,重试时可正确触发新建流,
		// 而 StartTime 在重试时不为零因此无法触发。
		if r.muxer == nil {
			err = r.createStream(sub.AudioReader.Value.WriteTime)
			if err != nil {
				return err
			}
			r.firstVideoFrame = true
		}
		r.Event.Duration = sub.AudioReader.AbsTime
		if sub.VideoReader == nil {
			if recordJob.Event != nil {
				err = checkEventRecordStop(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
			}
			if recordJob.RecConf.Fragment != 0 {
				err = checkFragment(sub.AudioReader)
				if err != nil {
					return err
				}
			}
		}
		sample := box.Sample{
			Timestamp: sub.AudioReader.AbsTime,
			Memory:    audio.Memory,
		}
		// 分片 createStream 异步执行期间将帧写入缓冲区
		if r.creating {
			select {
			case createErr := <-r.createDone:
				r.creating = false
				r.firstVideoFrame = true
				if createErr != nil {
					return createErr
				}
				if err = flushBuffer(); err != nil {
					return err
				}
			default:
				// ring buffer 的内存会被复用,必须深拷贝后再缓存,否则 createStream 完成后
				// flush 时读到的是已被覆盖的数据,导致文件损坏。
				var copiedMem gomem.Memory
				copiedMem.CopyFrom(&sample.Memory)
				sample.Memory = copiedMem
				r.sampleBuffer = append(r.sampleBuffer, bufferedSample{
					isAudio:  true,
					codecCtx: sub.AudioReader.Track.ICodecCtx,
					sample:   sample,
				})
				return nil
			}
		}
		if at == nil {
			at = sub.AudioReader.Track
			switch at.ICodecCtx.GetBase().(type) {
			case *codec.AACCtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_AAC)
				audioTrack = track
				track.ICodecCtx = at.ICodecCtx
			case *codec.PCMACtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_G711A)
				audioTrack = track
				track.ICodecCtx = at.ICodecCtx
			case *codec.PCMUCtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_G711U)
				audioTrack = track
				track.ICodecCtx = at.ICodecCtx
			}
		}
		return r.muxer.WriteSample(r.file, audioTrack, sample)
	}, func(video *VideoFrame) error {
		if r.muxer == nil {
			err = r.createStream(sub.VideoReader.Value.WriteTime)
			if err != nil {
				return err
			}
			r.firstVideoFrame = true
		}
		r.Event.Duration = sub.VideoReader.AbsTime
		if sub.VideoReader.Value.IDR {
			if recordJob.Event != nil {
				err = checkEventRecordStop(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
			}
			if recordJob.RecConf.Fragment != 0 {
				err = checkFragment(sub.VideoReader)
				if err != nil {
					return err
				}
			}
		}

		sample := box.Sample{
			Timestamp: sub.VideoReader.AbsTime,
			KeyFrame:  video.IDR,
			CTS:       video.GetCTS32(),
			Memory:    video.Memory,
		}
		// 如果是视频 I 帧，将参数集放在 I 帧前面一起写入
		//if r.firstVideoFrame && video.IDR {
		if video.IDR {
			// 创建包含参数集的 Memory
			var combinedMemory gomem.Memory
			var naluSizeLen int = 4
			var sps, pps, vps []byte

			switch ctx := video.ICodecCtx.GetBase().(type) {
			case *codec.H264Ctx:
				naluSizeLen = int(ctx.RecordInfo.LengthSizeMinusOne) + 1
				sps = ctx.SPS()
				pps = ctx.PPS()
				if len(sps) > 0 && len(pps) > 0 {
					// 写入 SPS
					sizeBuf := make([]byte, naluSizeLen)
					util.PutBE(sizeBuf, uint32(len(sps)))
					combinedMemory.Push(sizeBuf)
					combinedMemory.Push(sps)
					// 写入 PPS
					sizeBuf = make([]byte, naluSizeLen)
					util.PutBE(sizeBuf, uint32(len(pps)))
					combinedMemory.Push(sizeBuf)
					combinedMemory.Push(pps)
				}
			case *codec.H265Ctx:
				naluSizeLen = int(ctx.RecordInfo.LengthSizeMinusOne) + 1
				vps = ctx.VPS()
				sps = ctx.SPS()
				pps = ctx.PPS()
				if len(vps) > 0 && len(sps) > 0 && len(pps) > 0 {
					// 写入 VPS
					sizeBuf := make([]byte, naluSizeLen)
					util.PutBE(sizeBuf, uint32(len(vps)))
					combinedMemory.Push(sizeBuf)
					combinedMemory.Push(vps)
					// 写入 SPS
					sizeBuf = make([]byte, naluSizeLen)
					util.PutBE(sizeBuf, uint32(len(sps)))
					combinedMemory.Push(sizeBuf)
					combinedMemory.Push(sps)
					// 写入 PPS
					sizeBuf = make([]byte, naluSizeLen)
					util.PutBE(sizeBuf, uint32(len(pps)))
					combinedMemory.Push(sizeBuf)
					combinedMemory.Push(pps)
				}
			}
			// 将原始视频帧数据追加到参数集后面
			combinedMemory.Push(video.Memory.Buffers...)
			sample.Memory = combinedMemory
			r.firstVideoFrame = false
		} else if r.firstVideoFrame {
			r.firstVideoFrame = false
		}
		// 分片 createStream 异步执行期间将帧写入缓冲区
		if r.creating {
			select {
			case createErr := <-r.createDone:
				r.creating = false
				r.firstVideoFrame = true
				if createErr != nil {
					return createErr
				}
				if err = flushBuffer(); err != nil {
					return err
				}
			default:
				// ring buffer 的内存会被复用,必须深拷贝后再缓存,否则 createStream 完成后
				// flush 时读到的是已被覆盖的数据,导致文件损坏。
				var copiedMem gomem.Memory
				copiedMem.CopyFrom(&sample.Memory)
				sample.Memory = copiedMem
				r.sampleBuffer = append(r.sampleBuffer, bufferedSample{
					isAudio:  false,
					codecCtx: sub.VideoReader.Track.ICodecCtx,
					sample:   sample,
				})
				return nil
			}
		}
		if vt == nil {
			vt = sub.VideoReader.Track
			switch video.ICodecCtx.GetBase().(type) {
			case *codec.H264Ctx:
				track := r.muxer.AddTrack(box.MP4_CODEC_H264)
				videoTrack = track
				track.ICodecCtx = video.ICodecCtx
			case *codec.H265Ctx:
				track := r.muxer.AddTrack(box.MP4_CODEC_H265)
				videoTrack = track
				track.ICodecCtx = video.ICodecCtx
			}
		}
		//ctx := video.ICodecCtx.(pkg.IVideoCodecCtx)
		//if videoTrackCtx, ok := videoTrack.ICodecCtx.(pkg.IVideoCodecCtx); ok && videoTrackCtx != ctx {
		//	width, height := uint32(ctx.Width()), uint32(ctx.Height())
		//	oldWidth, oldHeight := uint32(videoTrackCtx.Width()), uint32(videoTrackCtx.Height())
		//	r.Info("ctx  changed, restarting recording",
		//		"old", fmt.Sprintf("%dx%d", oldWidth, oldHeight),
		//		"new", fmt.Sprintf("%dx%d", width, height))
		//	r.writeTailer(sub.VideoReader.Value.WriteTime)
		//	err = r.createStream(sub.VideoReader.Value.WriteTime)
		//	if err != nil {
		//		return nil
		//	}
		//	at, vt = nil, nil
		//	if vr := sub.VideoReader; vr != nil {
		//		vr.ResetAbsTime()
		//		vt = vr.Track
		//		switch video.ICodecCtx.GetBase().(type) {
		//		case *codec.H264Ctx:
		//			track := r.muxer.AddTrack(box.MP4_CODEC_H264)
		//			videoTrack = track
		//			track.ICodecCtx = video.ICodecCtx
		//		case *codec.H265Ctx:
		//			track := r.muxer.AddTrack(box.MP4_CODEC_H265)
		//			videoTrack = track
		//			track.ICodecCtx = video.ICodecCtx
		//		}
		//	}
		//	if ar := sub.AudioReader; ar != nil {
		//		ar.ResetAbsTime()
		//	}
		//}
		return r.muxer.WriteSample(r.file, videoTrack, sample)
	})
}
