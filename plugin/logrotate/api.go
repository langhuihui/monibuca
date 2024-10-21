package plugin_logrotate

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/phsym/console-slog"
	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/logrotate/pb"
)

func (h *LogRotatePlugin) List(context.Context, *emptypb.Empty) (*pb.ResponseFileInfo, error) {
	dir, err := os.Open(h.Path)
	if err == nil {
		var files []os.FileInfo
		if files, err = dir.Readdir(0); err == nil {
			var fileInfos []*pb.FileInfo
			for _, info := range files {
				fileInfos = append(fileInfos, &pb.FileInfo{
					Name: info.Name(), Size: info.Size(),
				})
			}
			return &pb.ResponseFileInfo{Files: fileInfos}, nil
		}
	}
	return nil, err
}

func (h *LogRotatePlugin) Get(_ context.Context, req *pb.RequestFileInfo) (res *pb.ResponseOpen, err error) {
	file, err1 := os.Open(filepath.Join(h.Path, req.FileName))
	if err1 == nil {
		defer file.Close()
		res = &pb.ResponseOpen{}
		content, err2 := io.ReadAll(file)
		if err2 == nil {
			res.Content = string(content)
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

func (l *LogRotatePlugin) API_tail(w http.ResponseWriter, r *http.Request) {
	writer := util.NewSSE(w, r.Context())
	h := console.NewHandler(writer, &console.HandlerOptions{NoColor: true})
	l.Server.AddLogHandler(h)
	<-r.Context().Done()
}
