package plugin_mp4

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/mcuadros/go-defaults"
	"google.golang.org/protobuf/types/known/emptypb"
	m7s "m7s.live/v5"
	"m7s.live/v5/pb"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
	mp4pb "m7s.live/v5/plugin/mp4/pb"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

type ContentPart struct {
	*os.File
	Start int64
	Size  int
}

func (p *MP4Plugin) download(w http.ResponseWriter, r *http.Request) {
	if p.DB == nil {
		http.Error(w, pkg.ErrNoDB.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	streamPath := r.PathValue("streamPath")

	query := r.URL.Query()
	var streams []m7s.RecordStream
	if id := query.Get("id"); id != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s.mp4", streamPath, id))
		p.DB.Find(&streams, "id=? AND stream_path=?", id, streamPath)
		if len(streams) == 0 {
			http.Error(w, "record not found", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, streams[0].FilePath)
		return
	}

	startTime, endTime, err := util.TimeRangeQueryParse(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.Info("download", "streamPath", streamPath, "start", startTime, "end", endTime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s_%s.mp4", streamPath, startTime.Format("20060102150405"), endTime.Format("20060102150405")))

	queryRecord := m7s.RecordStream{
		Mode: m7s.RecordModeAuto,
	}
	p.DB.Where(&queryRecord).Find(&streams, "end_time>? AND start_time<? AND stream_path=?", startTime, endTime, streamPath)
	muxer := mp4.NewMuxer(0)
	ftyp := box.CreateFTYPBox(box.TypeISOM, 0x200, box.TypeISOM, box.TypeISO2, box.TypeAVC1, box.TypeMP41)
	var n int64
	n, err = box.WriteTo(w, ftyp)
	if err != nil {
		return
	}
	muxer.CurrentOffset = n
	var lastTs, tsOffset int64
	var parts []*ContentPart
	sampleOffset := muxer.CurrentOffset + box.BasicBoxLen*2
	mdatOffset := sampleOffset
	var audioTrack, videoTrack *mp4.Track
	var file *os.File
	streamCount := len(streams)
	for i, stream := range streams {
		tsOffset = lastTs
		file, err = os.Open(stream.FilePath)
		if err != nil {
			return
		}
		p.Info("read", "file", file.Name())
		demuxer := mp4.NewDemuxer(file)
		err = demuxer.Demux()
		if err != nil {
			return
		}
		if i == 0 {
			for _, track := range demuxer.Tracks {
				t := muxer.AddTrack(track.Cid)
				t.ExtraData = track.ExtraData
				if track.Cid.IsAudio() {
					audioTrack = t
					t.SampleSize = track.SampleSize
					t.SampleRate = track.SampleRate
					t.ChannelCount = track.ChannelCount
				} else if track.Cid.IsVideo() {
					videoTrack = t
					t.Width = track.Width
					t.Height = track.Height
				}
			}
			startTimestamp := startTime.Sub(stream.StartTime).Milliseconds()
			var startSample *box.Sample
			if startSample, err = demuxer.SeekTime(uint64(startTimestamp)); err != nil {
				tsOffset = 0
				continue
			}
			tsOffset = -int64(startSample.DTS)
		}
		var part *ContentPart
		for track, sample := range demuxer.RangeSample {
			if i == streamCount-1 && int64(sample.DTS) > endTime.Sub(stream.StartTime).Milliseconds() {
				break
			}
			if part == nil {
				part = &ContentPart{
					File:  file,
					Start: sample.Offset,
				}
			}
			part.Size += sample.Size
			lastTs = int64(sample.DTS + uint64(tsOffset))
			fixSample := *sample
			fixSample.DTS += uint64(tsOffset)
			fixSample.PTS += uint64(tsOffset)
			fixSample.Offset += sampleOffset - part.Start
			if track.Cid.IsAudio() {
				audioTrack.AddSampleEntry(fixSample)
			} else if track.Cid.IsVideo() {
				videoTrack.AddSampleEntry(fixSample)
			}
		}
		if part != nil {
			sampleOffset += int64(part.Size)
			parts = append(parts, part)
		}
	}
	moovSize := muxer.GetMoovSize()
	for _, track := range muxer.Tracks {
		for i := range track.Samplelist {
			track.Samplelist[i].Offset += int64(moovSize)
		}
	}
	err = muxer.WriteMoov(w)
	if err != nil {
		return
	}
	var mdatBox = box.CreateBaseBox(box.TypeMDAT, uint64(sampleOffset-mdatOffset)+box.BasicBoxLen)
	var freeBox *box.FreeBox
	if mdatBox.HeaderSize() == box.BasicBoxLen {
		freeBox = box.CreateFreeBox(nil)
	}
	_, err = box.WriteTo(w, freeBox, mdatBox)
	if err != nil {
		return
	}
	var written, totalWritten int64
	for _, part := range parts {
		part.Seek(part.Start, io.SeekStart)
		written, err = io.CopyN(w, part.File, int64(part.Size))
		if err != nil {
			return
		}
		totalWritten += written
		part.Close()
	}
}

func (p *MP4Plugin) StartRecord(ctx context.Context, req *mp4pb.ReqStartRecord) (res *mp4pb.ResponseStartRecord, err error) {
	var recordExists bool
	res = &mp4pb.ResponseStartRecord{}
	p.Server.Records.Call(func() error {
		_, recordExists = p.Server.Records.Find(func(job *m7s.RecordJob) bool {
			return job.StreamPath == req.StreamPath && job.RecConf.FilePath == req.FilePath
		})
		return nil
	})
	if recordExists {
		err = pkg.ErrRecordExists
		return
	}
	p.Server.Streams.Call(func() error {
		if stream, ok := p.Server.Streams.Get(req.StreamPath); ok {
			recordConf := config.Record{
				Append:   false,
				Fragment: req.Fragment.AsDuration(),
				FilePath: req.FilePath,
			}
			job := p.Record(stream, recordConf, nil)
			res.Data = uint64(uintptr(unsafe.Pointer(job.GetTask())))
		}
		return nil
	})
	return
}

func (p *MP4Plugin) EventStart(ctx context.Context, req *mp4pb.ReqEventRecord) (res *mp4pb.ResponseEventRecord, err error) {
	beforeDuration := p.BeforeDuration
	afterDuration := p.AfterDuration
	res = &mp4pb.ResponseEventRecord{}
	if req.BeforeDuration != "" {
		beforeDuration, err = time.ParseDuration(req.BeforeDuration)
		if err != nil {
			p.Error("EventStart", "error", err)
		}
	}
	if req.AfterDuration != "" {
		afterDuration, err = time.ParseDuration(req.AfterDuration)
		if err != nil {
			p.Error("EventStart", "error", err)
		}
	}
	recorder := p.Meta.Recorder(config.Record{})
	var tmpJob *m7s.RecordJob
	p.Server.Records.Call(func() error {
		tmpJob, _ = p.Server.Records.Find(func(job *m7s.RecordJob) bool {
			return job.StreamPath == req.StreamPath
		})
		return nil
	})
	if tmpJob == nil { //为空表示没有正在进行的录制，也就是没有自动录像，则进行正常的事件录像
		p.Server.Streams.Call(func() error {
			if stream, ok := p.Server.Streams.Get(req.StreamPath); ok {
				recordConf := config.Record{
					Append:   false,
					Fragment: 0,
					FilePath: filepath.Join(p.EventRecordFilePath, stream.StreamPath, time.Now().Local().Format("2006-01-02-15-04-05")),
				}
				recordJob := recorder.GetRecordJob()
				recordJob.EventId = req.EventId
				recordJob.EventLevel = req.EventLevel
				recordJob.EventName = req.EventName
				recordJob.EventDesc = req.EventDesc
				recordJob.AfterDuration = afterDuration
				recordJob.BeforeDuration = beforeDuration
				recordJob.Mode = m7s.RecordModeEvent
				var subconfig config.Subscribe
				defaults.SetDefaults(&subconfig)
				subconfig.BufferTime = beforeDuration
				p.Record(stream, recordConf, &subconfig)
			}
			return nil
		})
	} else {
		if tmpJob.AfterDuration != 0 { //当前有事件录像正在录制，则更新该录像的结束时间
			tmpJob.AfterDuration = time.Duration(tmpJob.Subscriber.VideoReader.AbsTime)*time.Millisecond + afterDuration
		} else { //当前有自动录像正在录制，则生成事件录像的记录，而不去生成事件录像的文件
			recordStream := &m7s.RecordStream{
				StreamPath:     req.StreamPath,
				EventId:        req.EventId,
				EventLevel:     req.EventLevel,
				EventDesc:      req.EventDesc,
				EventName:      req.EventName,
				Mode:           m7s.RecordModeEvent,
				BeforeDuration: beforeDuration,
				AfterDuration:  afterDuration,
			}
			now := time.Now()
			startTime := now.Add(-beforeDuration)
			endTime := now.Add(afterDuration)
			recordStream.StartTime = startTime
			recordStream.EndTime = endTime
			if p.DB != nil {
				p.DB.Save(&recordStream)
			}
		}
	}
	return res, err
}

func (p *MP4Plugin) List(ctx context.Context, req *mp4pb.ReqRecordList) (resp *pb.ResponseList, err error) {
	globalReq := &pb.ReqRecordList{
		StreamPath: req.StreamPath,
		Range:      req.Range,
		Start:      req.Start,
		End:        req.End,
		PageNum:    req.PageNum,
		PageSize:   req.PageSize,
		Mode:       req.Mode,
		Type:       "mp4",
	}
	return p.Server.GetRecordList(ctx, globalReq)
}

func (p *MP4Plugin) Catalog(ctx context.Context, req *emptypb.Empty) (resp *pb.ResponseCatalog, err error) {
	return p.Server.GetRecordCatalog(ctx, &pb.ReqRecordCatalog{Type: "mp4"})
}

func (p *MP4Plugin) Delete(ctx context.Context, req *mp4pb.ReqRecordDelete) (resp *pb.ResponseDelete, err error) {
	globalReq := &pb.ReqRecordDelete{
		StreamPath: req.StreamPath,
		Ids:        req.Ids,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Range:      req.Range,
		Type:       "mp4",
	}
	return p.Server.DeleteRecord(ctx, globalReq)
}
