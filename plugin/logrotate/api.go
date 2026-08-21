package plugin_logrotate

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/phsym/console-slog"
	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/v5/pkg/collections"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/logrotate/pb"
)

func (h *LogRotatePlugin) List(context.Context, *emptypb.Empty) (*pb.ResponseFileInfo, error) {
	dir, err := os.Open(h.Path)
	if err != nil {
		return nil, err
	}
	files, err := dir.Readdir(0)
	if err != nil {
		return nil, err
	}
	fileInfos := collections.Map(files, func(info os.FileInfo) *pb.FileInfo {
		return &pb.FileInfo{Name: info.Name(), Size: info.Size()}
	})
	return &pb.ResponseFileInfo{Data: fileInfos}, nil
}

func (h *LogRotatePlugin) Get(_ context.Context, req *pb.RequestFileInfo) (res *pb.ResponseOpen, err error) {
	file, err1 := os.Open(filepath.Join(h.Path, req.FileName))
	if err1 == nil {
		defer file.Close()
		res = &pb.ResponseOpen{}
		content, err2 := io.ReadAll(file)
		if err2 == nil {
			res.Data = string(content)
		} else {
			err = err2
		}
	} else {
		err = err1
	}
	return
}

func (h *LogRotatePlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {

}

func (l *LogRotatePlugin) API_trail(w http.ResponseWriter, r *http.Request) {
	util.NewSSE(w, r.Context(), func(sse *util.SSE) {
		file, err := os.Open(filepath.Join(l.Path, "current.log"))
		if err == nil {
			io.Copy(sse, file)
			file.Close()
		}
		h := console.NewHandler(sse, &console.HandlerOptions{NoColor: true})
		l.Server.LogHandler.Add(h)
		<-r.Context().Done()
		l.Server.LogHandler.Remove(h)
	})
}
