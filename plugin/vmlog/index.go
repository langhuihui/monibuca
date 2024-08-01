package plugin_vmlog

import (
	"fmt"
	"log/slog"
	"m7s.live/m7s/v5"
	"net/http"
	//"m7s.live/m7s/v5/plugin/logrotate/pb"
)

// todo 配置
type VmLogPlugin struct {
	m7s.Plugin
	//Path      string `default:"./logs" desc:"日志文件存放目录"`
	//Size      uint64 `default:"1048576" desc:"日志文件大小，单位：字节"`
	//Days      int    `default:"1" desc:"日志文件保留天数"`
	//Formatter string `default:"2006-01-02T15" desc:"日志文件名格式"`
	//MaxFiles  uint64 `default:"7" desc:"最大日志文件数量"`
	//Level     string `default:"info" desc:"日志级别"`
	handler slog.Handler
}

var _ = m7s.InstallPlugin[VmLogPlugin]()

func (config *VmLogPlugin) OnInit() (err error) {
	config.handler, _ = NewVmLogHandler(nil, nil)
	config.AddLogHandler(config.handler)
	return
}

func (config *VmLogPlugin) Stop() {
	fmt.Print("VmLogPlugin OnClose")
}

func (config *VmLogPlugin) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	requestHandler(rw, r)
}
