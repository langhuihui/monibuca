package mp4

import (
	"bufio"
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

// progressDBInterval：fmp4 进行中切片周期刷新 EndTime 的节流间隔（REQ-MP4-001 方案 B / M1）
const progressDBInterval = 10 * time.Second

type WriteTrailerQueueTask struct {
	task.Work
}

var writeTrailerQueueTask WriteTrailerQueueTask

type writeTrailerTask struct {
	task.Task
	muxer    *Muxer
	file     storage.File
	filePath string
	// dbWrite 在文件完整写入后执行数据库更新，为 nil 时跳过（无 DB 或测试模式）。
	dbWrite func(tailJob task.IJob)
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
// 将 ftyp + free(optional) + moov + mdat 写入临时文件, 然后原子重命名替换原文件
// 优化：临时文件建在与原文件同一目录，写完后用 os.Rename 原子替换，
// 避免将整个临时文件再回写一遍（节省约 1/3 的磁盘 IO）。
func (t *writeTrailerTask) Run() (err error) {
	// FMP4：媒体已按 ftyp+moov+(moof+mdat)*+mfra 写出，无需普通 MP4 的「moov 挪到文件头」
	if t.muxer != nil && t.muxer.isFragment() {
		t.Info("write trailer fmp4 done")
		if t.file != nil {
			if err = t.file.Close(); err != nil {
				t.Error("close file", "err", err)
				return
			}
			t.file = nil
		}
		if t.dbWrite != nil {
			t.dbWrite(&writeTrailerQueueTask)
		}
		return nil
	}

	t.Info("write trailer")

	// 在与原文件相同目录创建临时文件，确保与目标在同一文件系统，支持原子 rename
	actualPath := t.file.Name()
	var temp *os.File
	temp, err = os.CreateTemp(filepath.Dir(actualPath), "*.mp4.tmp")
	if err != nil {
		t.Error("create temp file", "err", err)
		return
	}
	tempName := temp.Name()

	// 默认清理临时文件；rename 成功后将 committed 置 true 跳过删除
	committed := false
	defer func() {
		if !committed {
			os.Remove(tempName)
		}
	}()

	_, err = t.file.Seek(0, io.SeekStart)
	if err != nil {
		t.Error("seek file", "err", err)
		return
	}

	// 使用带缓冲的 writer 减少写入 syscall（moov 由大量小块组成）
	bw := bufio.NewWriterSize(temp, 1<<20) // 1 MB write buffer

	// 复制 mdat box 之前的内容
	_, err = io.CopyN(bw, t.file, int64(t.muxer.mdatOffset)-BeforeMdatData)
	if err != nil {
		t.Error("copy file", "err", err)
		return
	}
	for _, track := range t.muxer.Tracks {
		for i := range len(track.Samplelist) {
			track.Samplelist[i].Offset += int64(t.muxer.moov.Size())
		}
	}
	err = t.muxer.WriteMoov(bw)
	if err != nil {
		t.Error("rewrite with moov", "err", err)
		return
	}
	// 复制 mdat box
	_, err = io.CopyN(bw, t.file, int64(t.muxer.mdatSize)+BeforeMdatData)
	if err != nil {
		if err == pkg.ErrSkip {
			return task.ErrTaskComplete
		}
		t.Error("rewrite with mdat", "err", err)
		return
	}
	if err = bw.Flush(); err != nil {
		t.Error("flush temp file", "err", err)
		return
	}
	if err = t.file.Close(); err != nil {
		t.Error("close file", "err", err)
		return
	}
	if err = temp.Close(); err != nil {
		t.Error("close temp file", "err", err)
		return
	}
	// 原子重命名替换原文件，避免将临时文件整体回写（节省约 1/3 的磁盘 IO）
	if err = os.Rename(tempName, actualPath); err != nil {
		t.Error("rename temp file", "err", err)
		return
	}
	committed = true
	// 文件已完整写入（moov 在头部），此时才将记录写入数据库。
	if t.dbWrite != nil {
		t.dbWrite(&writeTrailerQueueTask)
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
	muxer            *Muxer
	file             storage.File
	firstVideoFrame  bool // 标记是否是第一个视频帧
	creating         bool
	createDone       chan error
	sampleBuffer     []bufferedSample
	audioTrack       *Track // FMP4 预置轨，避免与 WriteMoov 竞态
	videoTrack       *Track
	lastProgressDB   time.Time // fmp4 上次刷新 EndTime 的墙钟，用于节流
	progressClosedID uint      // 已交给 writeTrailer 定稿的记录 ID，禁止再被进度刷新回退
}

// maybeFlushProgress 节流更新进行中 fmp4 记录的 EndTime/Duration，供 list/点播查询命中。
// 普通 mp4 不刷新（进行中文件尚无 moov，写入 EndTime 会造成「可见但不可播」）。
// Confirmed via 寸止: REQ-MP4-001 方案 B / M1+M4
func (r *Recorder) maybeFlushProgress(end time.Time, duration uint32) {
	if r.creating || r.Event.Type != "fmp4" || r.Event.ID == 0 {
		return
	}
	id := r.Event.ID
	// M4：已 writeTailer 的记录禁止再刷，避免与 deferred Save 竞态把 EndTime 写回去
	if id == r.progressClosedID {
		return
	}
	db := r.RecordJob.Plugin.DB
	if db == nil || r.RecordJob.RecConf.Mode == config.RecordModeTest {
		return
	}
	// 首次立即刷新；之后按 progressDBInterval 节流
	if !r.lastProgressDB.IsZero() && end.Sub(r.lastProgressDB) < progressDBInterval {
		return
	}
	r.lastProgressDB = end
	r.Event.EndTime = end
	r.Event.Duration = duration
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 仅允许 end_time 单调前进，防止迟到的 Update 覆盖定稿值
	if result := db.WithContext(dbCtx).Model(&m7s.RecordStream{}).
		Where("id = ? AND end_time < ?", id, end).
		Updates(map[string]any{
			"end_time": end,
			"duration": duration,
		}); result.Error != nil {
		r.Warn("db progress update failed", "id", id, "err", result.Error)
	}
}

func (r *Recorder) writeTailer(end time.Time) {
	// M4：先锁定当前记录 ID，阻断后续 maybeFlushProgress（含在途 goroutine 语义上的迟到写）
	if r.Event.ID != 0 {
		r.progressClosedID = r.Event.ID
	}
	// WriteTailDeferred 仅设置 EndTime 并返回延迟 DB 写入闭包，不立即写库。
	// DB 写入将在 writeTrailerTask.Run() 成功后执行，确保文件可播放后才入库。
	dbWrite := r.WriteTailDeferred(end)
	writeTrailerQueueTask.AddTask(&writeTrailerTask{
		muxer:    r.muxer,
		file:     r.file,
		filePath: r.Event.FilePath,
		dbWrite:  dbWrite,
	}, r.Logger.With("filePath", r.Event.FilePath, "streamPath", r.Event.StreamPath))
}

var CustomFileName = func(job *m7s.RecordJob) string {
	now := time.Now()
	return filepath.Join(job.RecConf.FilePath, fmt.Sprintf("%s_%09d.mp4", time.Now().Local().Format("2006-01-02-15-04-05"), now.Nanosecond()))
}

func (r *Recorder) createStream(start time.Time) (err error) {
	if r.RecordJob.RecConf.Type == "" {
		r.RecordJob.RecConf.Type = "mp4"
	}
	r.lastProgressDB = time.Time{}
	t0 := time.Now()
	err = r.CreateStream(start, CustomFileName)
	r.Info("createStream step1 CreateStream", "elapsed", time.Since(t0))
	if err != nil {
		return
	}
	// fmp4：开片后记一次进度时间戳，避免紧接着的 maybeFlushProgress 重复写库
	if r.Event.Type == "fmp4" && !r.Event.EndTime.IsZero() {
		r.lastProgressDB = r.Event.EndTime
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
	r.file, err = st.CreateFile(r.Context, r.Event.FilePath)
	r.Info("createStream step2 CreateFile", "elapsed", time.Since(t1), "path", r.Event.FilePath)
	if err != nil {
		return
	}

	r.audioTrack, r.videoTrack = nil, nil
	if r.Event.Type == "fmp4" {
		r.muxer = NewMuxerWithStreamPath(FLAG_FRAGMENT, r.Event.StreamPath)
	} else {
		r.muxer = NewMuxerWithStreamPath(0, r.Event.StreamPath)
	}
	t2 := time.Now()
	err = r.muxer.WriteInitSegment(r.file)
	r.Info("createStream step3 WriteInitSegment", "elapsed", time.Since(t2))
	if err != nil {
		return
	}
	// FMP4：在首个 moof 前写入含 mvex 的 moov（与 HTTP 点播路径一致）
	if r.muxer.isFragment() {
		t3 := time.Now()
		if err = r.initFragmentTracks(); err != nil {
			r.Error("createStream initFragmentTracks", "err", err)
			return
		}
		if err = r.muxer.WriteMoov(r.file); err != nil {
			r.Error("createStream WriteMoov", "err", err)
			return
		}
		r.Info("createStream step4 WriteMoov", "elapsed", time.Since(t3))
	}
	r.Info("createStream total", "elapsed", time.Since(t0))
	r.SetDescription("startTime", start.Format("2006-01-02 15:04:05"))
	return
}

// initFragmentTracks 按 Publisher 已就绪轨预置 FMP4 Track，保证 WriteMoov 含全部轨
func (r *Recorder) initFragmentTracks() (err error) {
	sub := r.RecordJob.Subscriber
	if sub == nil || sub.Publisher == nil {
		return nil
	}
	pub := sub.Publisher
	if pub.HasVideoTrack() && sub.SubVideo {
		v := pub.VideoTrack.AVTrack
		if err = v.WaitReady(); err != nil {
			return
		}
		var codecID box.MP4_CODEC_TYPE
		switch v.ICodecCtx.FourCC() {
		case codec.FourCC_H264:
			codecID = box.MP4_CODEC_H264
		case codec.FourCC_H265:
			codecID = box.MP4_CODEC_H265
		default:
			r.Warn("fmp4 skip unsupported video codec", "fourcc", v.ICodecCtx.FourCC())
		}
		if codecID != 0 {
			track := r.muxer.AddTrack(codecID)
			track.ICodecCtx = v.ICodecCtx
			r.videoTrack = track
		}
	}
	if pub.HasAudioTrack() && sub.SubAudio {
		a := pub.AudioTrack.AVTrack
		if err = a.WaitReady(); err != nil {
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
		default:
			r.Warn("fmp4 skip unsupported audio codec", "fourcc", a.ICodecCtx.FourCC())
		}
		if codecID != 0 {
			track := r.muxer.AddTrack(codecID)
			track.ICodecCtx = a.ICodecCtx
			r.audioTrack = track
		}
	}
	return nil
}

func (r *Recorder) Dispose() {
	r.audioTrack, r.videoTrack = nil, nil
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
			r.audioTrack, r.videoTrack = nil, nil
			go func() {
				createErr := r.createStream(startTime)
				r.Info("check fragment end async", "err", createErr)
				r.createDone <- createErr
			}()
			at, vt = nil, nil
			audioTrack, videoTrack = nil, nil
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
		audioTrack, videoTrack = r.audioTrack, r.videoTrack
		for _, bs := range r.sampleBuffer {
			if bs.isAudio {
				if at == nil {
					at = sub.AudioReader.Track
					if audioTrack == nil {
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
						r.audioTrack = audioTrack
					}
				}
				if err := r.muxer.WriteSample(r.file, audioTrack, bs.sample); err != nil {
					return err
				}
			} else {
				if vt == nil {
					vt = sub.VideoReader.Track
					if videoTrack == nil {
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
						r.videoTrack = videoTrack
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
			if r.audioTrack != nil {
				audioTrack = r.audioTrack
			} else {
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
				r.audioTrack = audioTrack
			}
		}
		if err = r.muxer.WriteSample(r.file, audioTrack, sample); err != nil {
			return err
		}
		// 仅音轨录制时也推进进行中 EndTime
		if sub.VideoReader == nil {
			r.maybeFlushProgress(sub.AudioReader.Value.WriteTime, sub.AudioReader.AbsTime)
		}
		return nil
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
			if r.videoTrack != nil {
				videoTrack = r.videoTrack
			} else {
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
				r.videoTrack = videoTrack
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
		if err = r.muxer.WriteSample(r.file, videoTrack, sample); err != nil {
			return err
		}
		r.maybeFlushProgress(sub.VideoReader.Value.WriteTime, sub.VideoReader.AbsTime)
		return nil
	})
}
