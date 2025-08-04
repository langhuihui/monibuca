package plugin_flv

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"google.golang.org/protobuf/types/known/emptypb"
	m7s "m7s.live/v5"
	"m7s.live/v5/pb"
	"m7s.live/v5/pkg/util"
	flvpb "m7s.live/v5/plugin/flv/pb"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

func (p *FLVPlugin) List(ctx context.Context, req *flvpb.ReqRecordList) (resp *pb.RecordResponseList, err error) {
	globalReq := &pb.ReqRecordList{
		StreamPath: req.StreamPath,
		Range:      req.Range,
		Start:      req.Start,
		End:        req.End,
		PageNum:    req.PageNum,
		PageSize:   req.PageSize,
		Type:       "flv",
	}
	return p.Server.GetRecordList(ctx, globalReq)
}

func (p *FLVPlugin) Catalog(ctx context.Context, req *emptypb.Empty) (resp *pb.ResponseCatalog, err error) {
	return p.Server.GetRecordCatalog(ctx, &pb.ReqRecordCatalog{Type: "flv"})
}

func (p *FLVPlugin) Delete(ctx context.Context, req *flvpb.ReqRecordDelete) (resp *pb.ResponseDelete, err error) {
	globalReq := &pb.ReqRecordDelete{
		StreamPath: req.StreamPath,
		Ids:        req.Ids,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Range:      req.Range,
		Type:       "flv",
	}
	return p.Server.DeleteRecord(ctx, globalReq)
}

func (plugin *FLVPlugin) Download_(w http.ResponseWriter, r *http.Request) {
	// 解析请求参数
	params, err := plugin.parseRequestParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	plugin.Info("download", "stream", params.streamPath, "start", params.startTime, "end", params.endTime)

	// 从数据库查询录像记录
	recordStreams, err := plugin.queryRecordStreams(params)
	if err != nil {
		plugin.Error("Failed to query record streams", "err", err)
		http.Error(w, "Database query failed", http.StatusInternalServerError)
		return
	}

	// 构建文件信息列表
	fileInfoList, found := plugin.buildFileInfoList(recordStreams, params.startTime, params.endTime)
	if !found || len(fileInfoList) == 0 {
		plugin.Warn("No records found", "stream", params.streamPath, "start", params.startTime, "end", params.endTime)
		http.NotFound(w, r)
		return
	}

	// 根据记录类型选择处理方式
	if plugin.hasOnlyMp4Records(fileInfoList) {
		// 过滤MP4文件并转换为FLV
		mp4FileList := plugin.filterMp4Files(fileInfoList)
		if len(mp4FileList) == 0 {
			plugin.Warn("No valid MP4 files after filtering", "stream", params.streamPath)
			http.NotFound(w, r)
			return
		}
		plugin.processMp4ToFlv(w, r, mp4FileList, params)
	} else {
		// 过滤FLV文件并处理
		flvFileList := plugin.filterFlvFiles(fileInfoList)
		if len(flvFileList) == 0 {
			plugin.Warn("No valid FLV files after filtering", "stream", params.streamPath)
			http.NotFound(w, r)
			return
		}
		plugin.processFlvFiles(w, r, flvFileList, params)
	}
}

func (plugin *FLVPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/jessica/{streamPath}": plugin.jessica,
	}
}

// /jessica/{streamPath}
func (plugin *FLVPlugin) jessica(rw http.ResponseWriter, r *http.Request) {
	subscriber, err := plugin.Subscribe(r.Context(), r.PathValue("streamPath"))
	defer func() {
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
	}()
	if err != nil {
		return
	}
	var conn net.Conn
	conn, err = subscriber.CheckWebSocket(rw, r)
	if err != nil {
		return
	}
	if conn == nil {
		err = errors.New("no websocket connection.")
		return
	}
	var _sendBuffer = net.Buffers{}
	sendBuffer := _sendBuffer
	var head [5]byte
	write := func(typ byte, ts uint32, mem util.Memory) (err error) {
		head[0] = typ
		binary.BigEndian.PutUint32(head[1:], ts)
		err = ws.WriteHeader(conn, ws.Header{
			Fin:    true,
			OpCode: ws.OpBinary,
			Length: int64(mem.Size + 5),
		})
		if err != nil {
			return
		}
		sendBuffer = append(_sendBuffer, head[:])
		sendBuffer = append(sendBuffer, mem.Buffers...)
		if plugin.GetCommonConf().WriteTimeout > 0 {
			conn.SetWriteDeadline(time.Now().Add(plugin.GetCommonConf().WriteTimeout))
		}
		_, err = sendBuffer.WriteTo(conn)
		return
	}

	m7s.PlayBlock(subscriber, func(audio *rtmp.AudioFrame) (err error) {
		return write(1, audio.GetTS32(), audio.Memory)
	}, func(video *rtmp.VideoFrame) (err error) {
		return write(2, video.GetTS32(), video.Memory)
	})
}
