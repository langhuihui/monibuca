package plugin_mp4

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	mp4 "m7s.live/m7s/v5/plugin/mp4/pkg"
	"m7s.live/m7s/v5/plugin/mp4/pkg/box"
)

type ContentPart struct {
	*os.File
	Start int64
	Size  int
}

func (p *MP4Plugin) download(w http.ResponseWriter, r *http.Request) {
	filePath := r.PathValue("filePath")
	query := r.URL.Query()
	rangeStr := strings.Split(query.Get("range"), "~")
	var startTime, endTime time.Time
	if len(rangeStr) != 2 {
		http.NotFound(w, r)
		return
	}
	var err error
	startTime, err = util.TimeQueryParse(rangeStr[0])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	endTime, err = util.TimeQueryParseRefer(rangeStr[1], startTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	//timeRange := endTime.Sub(startTime)
	p.Info("download", "filePath", filePath, "start", startTime, "end", endTime)
	var streams []m7s.RecordStream
	p.DB.Find(&streams, "end_time>? AND start_time<? AND file_path=?", startTime, endTime, filePath)
	muxer := mp4.NewMuxer(0)
	var n int
	n, err = w.Write(box.MakeFtypBox(box.TypeISOM, 0x200, box.TypeISOM, box.TypeISO2, box.TypeAVC1, box.TypeMP41))
	if err != nil {
		return
	}
	muxer.CurrentOffset = int64(n)
	var lastTs, tsOffset int64
	var parts []*ContentPart
	sampleOffset := muxer.CurrentOffset + box.BasicBoxLen*2
	mdatOffset := sampleOffset
	var audioTrack, videoTrack *mp4.Track
	for i, stream := range streams {
		tsOffset = lastTs
		file, err := os.Open(filepath.Join(filePath, fmt.Sprintf("%d.mp4", stream.ID)))
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
				continue
			}
			tsOffset = -int64(startSample.DTS)
		}
		var part *ContentPart
		for track, sample := range demuxer.RangeSample {
			if endTime.After(stream.StartTime) && int64(sample.DTS) > endTime.Sub(stream.StartTime).Milliseconds() {
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
	var mdatBox = box.MediaDataBox(sampleOffset - mdatOffset)
	boxLen, buf := mdatBox.Encode()
	if boxLen == box.BasicBoxLen*2 {
		w.Write(buf)
	} else {
		freeBox := box.NewBasicBox(box.TypeFREE)
		freeBox.Size = box.BasicBoxLen
		_, free := freeBox.Encode()
		w.Write(free)
		w.Write(buf)
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
