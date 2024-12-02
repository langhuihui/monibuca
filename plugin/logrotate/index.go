package plugin_logrotate

import (
	"io"
	"log/slog"

	"github.com/alchemy/rotoslog"

	"github.com/phsym/console-slog"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/plugin/logrotate/pb"
)

type LogRotatePlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	Path      string `default:"./logs" desc:"日志文件存放目录"`
	Size      uint64 `default:"1048576" desc:"日志文件大小，单位：字节"`
	Days      int    `default:"1" desc:"日志文件保留天数"`
	Formatter string `default:"2006-01-02T15" desc:"日志文件名格式"`
	MaxFiles  uint64 `default:"7" desc:"最大日志文件数量"`
	Level     string `default:"info" desc:"日志级别"`
	handler   slog.Handler
}

var _ = m7s.InstallPlugin[LogRotatePlugin](&pb.Api_ServiceDesc, pb.RegisterApiHandler)

func (config *LogRotatePlugin) OnInit() (err error) {
	builder := func(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return console.NewHandler(w, &console.HandlerOptions{NoColor: true, Level: pkg.ParseLevel(config.Level), TimeFormat: "2006-01-02 15:04:05.000"})
	}
	config.handler, err = rotoslog.NewHandler(rotoslog.LogHandlerBuilder(builder), rotoslog.LogDir(config.Path), rotoslog.MaxFileSize(config.Size), rotoslog.DateTimeLayout(config.Formatter), rotoslog.MaxRotatedFiles(config.MaxFiles))
	if err == nil {
		config.Server.LogHandler.Add(config.handler)
	}
	return
}
