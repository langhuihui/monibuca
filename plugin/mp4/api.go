package plugin_mp4

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	Start  int64
	Size   int
	boxies []box.IBox
}

func (p *MP4Plugin) downloadSingleFile(stream *m7s.RecordStream, flag mp4.Flag, w http.ResponseWriter, r *http.Request) {
	if flag == 0 {
		http.ServeFile(w, r, stream.FilePath)
	} else if flag == mp4.FLAG_FRAGMENT {
		file, err := os.Open(stream.FilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.Info("read", "file", file.Name())
		demuxer := mp4.NewDemuxer(file)
		err = demuxer.Demux()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var trackMap = make(map[box.MP4_CODEC_TYPE]*mp4.Track)
		muxer := mp4.NewMuxer(mp4.FLAG_FRAGMENT)
		for _, track := range demuxer.Tracks {
			t := muxer.AddTrack(track.Cid)
			t.ExtraData = track.ExtraData
			trackMap[track.Cid] = t
			if track.Cid.IsAudio() {
				t.SampleSize = track.SampleSize
				t.SampleRate = track.SampleRate
				t.ChannelCount = track.ChannelCount
			} else if track.Cid.IsVideo() {
				t.Width = track.Width
				t.Height = track.Height
			}
		}
		moov := muxer.MakeMoov()
		var parts []*ContentPart
		var part *ContentPart
		for track, sample := range demuxer.RangeSample {
			if part == nil {
				part = &ContentPart{
					File:  file,
					Start: sample.Offset,
				}
				parts = append(parts, part)
			}
			fixSample := *sample
			part.Seek(sample.Offset, io.SeekStart)
			fixSample.Data = make([]byte, sample.Size)
			part.Read(fixSample.Data)
			moof, mdat := muxer.CreateFlagment(trackMap[track.Cid], fixSample)
			if moof != nil {
				part.boxies = append(part.boxies, moof, mdat)
				part.Size += int(moof.Size() + mdat.Size())
			}
		}
		var children []box.IBox
		var totalSize uint64
		ftyp := muxer.CreateFTYPBox()
		children = append(children, ftyp, moov)
		totalSize += uint64(ftyp.Size() + moov.Size())
		for _, part := range parts {
			totalSize += uint64(part.Size)
			children = append(children, part.boxies...)
			part.Close()
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", totalSize))
		_, err = box.WriteTo(w, children...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (p *MP4Plugin) download(w http.ResponseWriter, r *http.Request) {
	if p.DB == nil {
		http.Error(w, pkg.ErrNoDB.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	streamPath := r.PathValue("streamPath")
	var flag mp4.Flag
	if strings.HasSuffix(streamPath, ".fmp4") {
		flag = mp4.FLAG_FRAGMENT
		streamPath = strings.TrimSuffix(streamPath, ".fmp4")
	} else {
		streamPath = strings.TrimSuffix(streamPath, ".mp4")
	}
	query := r.URL.Query()
	var streams []m7s.RecordStream
	if id := query.Get("id"); id != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s.mp4", streamPath, id))
		p.DB.Find(&streams, "id=? AND stream_path=?", id, streamPath)
		if len(streams) == 0 {
			http.Error(w, "record not found", http.StatusNotFound)
			return
		}
		p.downloadSingleFile(&streams[0], flag, w, r)
		return
	}
	// 合并多个 mp4
	startTime, endTime, err := util.TimeRangeQueryParse(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.Info("download", "streamPath", streamPath, "start", startTime, "end", endTime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s_%s.mp4", streamPath, startTime.Format("20060102150405"), endTime.Format("20060102150405")))

	queryRecord := m7s.RecordStream{
		Mode: m7s.RecordModeAuto,
		Type: "mp4",
	}
	p.DB.Where(&queryRecord).Find(&streams, "end_time>? AND start_time<? AND stream_path=?", startTime, endTime, streamPath)
	muxer := mp4.NewMuxer(flag)
	ftyp := muxer.CreateFTYPBox()
	n := ftyp.Size()
	muxer.CurrentOffset = int64(n)
	var lastTs, tsOffset int64
	var parts []*ContentPart
	sampleOffset := muxer.CurrentOffset + mp4.BeforeMdatData
	mdatOffset := sampleOffset
	var audioTrack, videoTrack *mp4.Track
	var file *os.File
	var moov box.IBox
	streamCount := len(streams)

	// Track ExtraData history for each track
	type TrackHistory struct {
		Track     *mp4.Track
		ExtraData []byte
	}
	var audioHistory, videoHistory []TrackHistory
	addAudioTrack := func(track *mp4.Track) {
		t := muxer.AddTrack(track.Cid)
		t.ExtraData = track.ExtraData
		t.SampleSize = track.SampleSize
		t.SampleRate = track.SampleRate
		t.ChannelCount = track.ChannelCount
		if len(audioHistory) > 0 {
			t.Samplelist = audioHistory[len(audioHistory)-1].Track.Samplelist
		}
		audioTrack = t
		audioHistory = append(audioHistory, TrackHistory{Track: t, ExtraData: track.ExtraData})
	}

	addVideoTrack := func(track *mp4.Track) {
		t := muxer.AddTrack(track.Cid)
		t.ExtraData = track.ExtraData
		t.Width = track.Width
		t.Height = track.Height
		if len(videoHistory) > 0 {
			t.Samplelist = videoHistory[len(videoHistory)-1].Track.Samplelist
		}
		videoTrack = t
		videoHistory = append(videoHistory, TrackHistory{Track: t, ExtraData: track.ExtraData})
	}

	addTrack := func(track *mp4.Track) {
		var lastAudioTrack, lastVideoTrack *TrackHistory
		if len(audioHistory) > 0 {
			lastAudioTrack = &audioHistory[len(audioHistory)-1]
		}
		if len(videoHistory) > 0 {
			lastVideoTrack = &videoHistory[len(videoHistory)-1]
		}
		if track.Cid.IsAudio() {
			if lastAudioTrack == nil {
				addAudioTrack(track)
			} else if !bytes.Equal(lastAudioTrack.ExtraData, track.ExtraData) {
				for _, history := range audioHistory {
					if bytes.Equal(history.ExtraData, track.ExtraData) {
						audioTrack = history.Track
						audioTrack.Samplelist = audioHistory[len(audioHistory)-1].Track.Samplelist
						return
					}
				}
				addAudioTrack(track)
			}
		} else if track.Cid.IsVideo() {
			if lastVideoTrack == nil {
				addVideoTrack(track)
			} else if !bytes.Equal(lastVideoTrack.ExtraData, track.ExtraData) {
				for _, history := range videoHistory {
					if bytes.Equal(history.ExtraData, track.ExtraData) {
						videoTrack = history.Track
						videoTrack.Samplelist = videoHistory[len(videoHistory)-1].Track.Samplelist
						return
					}
				}
				addVideoTrack(track)
			}
		}
	}

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
		trackCount := len(demuxer.Tracks)
		if i == 0 || flag == mp4.FLAG_FRAGMENT {
			for _, track := range demuxer.Tracks {
				addTrack(track)
			}
		}
		if trackCount != len(muxer.Tracks) {
			if flag == mp4.FLAG_FRAGMENT {
				moov = muxer.MakeMoov()
			}
		}
		if i == 0 {
			startTimestamp := startTime.Sub(stream.StartTime).Milliseconds()
			var startSample *box.Sample
			if startSample, err = demuxer.SeekTime(uint64(startTimestamp)); err != nil {
				tsOffset = 0
				continue
			}
			tsOffset = -int64(startSample.Timestamp)
		}
		var part *ContentPart
		for track, sample := range demuxer.RangeSample {
			if i == streamCount-1 && int64(sample.Timestamp) > endTime.Sub(stream.StartTime).Milliseconds() {
				break
			}
			if part == nil {
				part = &ContentPart{
					File:  file,
					Start: sample.Offset,
				}
			}
			lastTs = int64(sample.Timestamp + uint32(tsOffset))
			fixSample := *sample
			fixSample.Timestamp += uint32(tsOffset)
			if flag == 0 {
				fixSample.Offset = sampleOffset + (fixSample.Offset - part.Start)
				part.Size += sample.Size
				if track.Cid.IsAudio() {
					audioTrack.AddSampleEntry(fixSample)
				} else if track.Cid.IsVideo() {
					videoTrack.AddSampleEntry(fixSample)
				}
			} else {
				part.Seek(sample.Offset, io.SeekStart)
				fixSample.Data = make([]byte, sample.Size)
				part.Read(fixSample.Data)
				var moof, mdat box.IBox
				if track.Cid.IsAudio() {
					moof, mdat = muxer.CreateFlagment(audioTrack, fixSample)
				} else if track.Cid.IsVideo() {
					moof, mdat = muxer.CreateFlagment(videoTrack, fixSample)
				}
				if moof != nil {
					part.boxies = append(part.boxies, moof, mdat)
					part.Size += int(moof.Size() + mdat.Size())
				}
			}
		}
		if part != nil {
			sampleOffset += int64(part.Size)
			parts = append(parts, part)
		}
	}

	if flag == 0 {
		moovSize := muxer.MakeMoov().Size()
		dataSize := uint64(sampleOffset - mdatOffset)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", uint64(sampleOffset)+moovSize))
		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
			}
		}
		mdatBox := box.CreateBaseBox(box.TypeMDAT, dataSize+box.BasicBoxLen)

		var freeBox *box.FreeBox
		if mdatBox.HeaderSize() == box.BasicBoxLen {
			freeBox = box.CreateFreeBox(nil)
		}

		var written, totalWritten int64

		totalWritten, err = box.WriteTo(w, ftyp, muxer.MakeMoov(), freeBox, mdatBox)
		if err != nil {
			return
		}

		for _, part := range parts {
			part.Seek(part.Start, io.SeekStart)
			written, err = io.CopyN(w, part.File, int64(part.Size))
			if err != nil {
				return
			}
			totalWritten += written
			part.Close()
		}
	} else {
		var children []box.IBox
		var totalSize uint64
		children = append(children, ftyp, moov)
		totalSize += uint64(ftyp.Size() + moov.Size())
		for _, part := range parts {
			totalSize += uint64(part.Size)
			children = append(children, part.boxies...)
			part.Close()
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", totalSize))
		_, err = box.WriteTo(w, children...)
		if err != nil {
			return
		}
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
	//recorder := p.Meta.Recorder(config.Record{})
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
				//recordJob := recorder.GetRecordJob()
				var subconfig config.Subscribe
				defaults.SetDefaults(&subconfig)
				subconfig.BufferTime = beforeDuration
				recordJob := p.Record(stream, recordConf, &subconfig)
				recordJob.EventId = req.EventId
				recordJob.EventLevel = req.EventLevel
				recordJob.EventName = req.EventName
				recordJob.EventDesc = req.EventDesc
				recordJob.AfterDuration = afterDuration
				recordJob.BeforeDuration = beforeDuration
				recordJob.Mode = m7s.RecordModeEvent
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
				Type:           "mp4",
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
		EventLevel: req.EventLevel,
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
