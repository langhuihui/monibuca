package record

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	. "github.com/langhuihui/monibuca/monica"
)

var config = struct {
	Path string
}{}
var recordings = sync.Map{}

type FlvFileInfo struct {
	Path     string
	Size     int64
	Duration uint32
}

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "RecordFlv",
		Type:   PLUGIN_SUBSCRIBER,
		Config: &config,
		Run:    run,
	})
}
func run() {
	OnSubscribeHooks.AddHook(onSubscribe)
	if !strings.HasSuffix(config.Path, "/") {
		config.Path = config.Path + "/"
	}
	http.HandleFunc("/api/record/flv/list", func(writer http.ResponseWriter, r *http.Request) {
		if files, err := tree(config.Path, 0); err == nil {
			var bytes []byte
			if bytes, err = json.Marshal(files); err == nil {
				writer.Write(bytes)
			} else {
				writer.Write([]byte("{\"err\":\"" + err.Error() + "\"}"))
			}
		} else {
			writer.Write([]byte("{\"err\":\"" + err.Error() + "\"}"))
		}
	})
	http.HandleFunc("/api/record/flv", func(writer http.ResponseWriter, r *http.Request) {
		if streamPath := r.URL.Query().Get("streamPath"); streamPath != "" {
			if err := SaveFlv(streamPath, r.URL.Query().Get("append") != ""); err != nil {
				writer.Write([]byte(err.Error()))
			} else {
				writer.Write([]byte("success"))
			}
		} else {
			writer.Write([]byte("no streamPath"))
		}
	})

	http.HandleFunc("/api/record/flv/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if streamPath := r.URL.Query().Get("streamPath"); streamPath != "" {
			filePath := config.Path + streamPath + ".flv"
			if stream, ok := recordings.Load(filePath); ok {
				output := stream.(*OutputStream)
				output.Close()
				w.Write([]byte("success"))
			} else {
				w.Write([]byte("no query stream"))
			}
		} else {
			w.Write([]byte("no such stream"))
		}
	})
	http.HandleFunc("/api/record/flv/play", func(writer http.ResponseWriter, r *http.Request) {
		if streamPath := r.URL.Query().Get("streamPath"); streamPath != "" {
			if err := PublishFlvFile(streamPath); err != nil {
				writer.Write([]byte(err.Error()))
			} else {
				writer.Write([]byte("success"))
			}
		} else {
			writer.Write([]byte("no streamPath"))
		}
	})
}
func onSubscribe(s *OutputStream) {
	filePath := config.Path + s.StreamPath + ".flv"
	if s.Publisher == nil && PathExists(filePath) {
		PublishFlvFile(s.StreamPath)
	}
}
func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}
func tree(dstPath string, level int) (files []*FlvFileInfo, err error) {
	var dstF *os.File
	dstF, err = os.Open(dstPath)
	if err != nil {
		return
	}
	defer dstF.Close()
	fileInfo, err := dstF.Stat()
	if err != nil {
		return
	}
	if !fileInfo.IsDir() { //如果dstF是文件
		if path.Ext(fileInfo.Name()) == ".flv" {
			files = append(files, &FlvFileInfo{
				Path:     strings.TrimPrefix(strings.TrimPrefix(dstPath, config.Path), "/"),
				Size:     fileInfo.Size(),
				Duration: getDuration(dstF),
			})
		}
		return
	} else { //如果dstF是文件夹
		var dir []os.FileInfo
		dir, err = dstF.Readdir(0) //获取文件夹下各个文件或文件夹的fileInfo
		if err != nil {
			return
		}
		for _, fileInfo = range dir {
			var _files []*FlvFileInfo
			_files, err = tree(dstPath+"/"+fileInfo.Name(), level+1)
			if err != nil {
				return
			}
			files = append(files, _files...)
		}
		return
	}

}
