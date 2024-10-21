package plugin_debug

import (
	myproc "github.com/cloudwego/goref/pkg/proc"
	"github.com/go-delve/delve/pkg/config"
	"github.com/go-delve/delve/service/debugger"
	"io"
	"m7s.live/v5"
	"net/http"
	"net/http/pprof"
	"os"
	runtimePPROF "runtime/pprof"
	"strings"
	"time"
)

var _ = m7s.InstallPlugin[DebugPlugin]()
var conf, _ = config.LoadConfig()

type DebugPlugin struct {
	m7s.Plugin
	ProfileDuration time.Duration `default:"10s" desc:"profile持续时间"`
	Profile         string        `desc:"采集profile存储文件"`
	ChartPeriod     time.Duration `default:"1s" desc:"图表更新周期"`
	Grfout          string        `default:"grf.out" desc:"grf输出文件"`
}

type WriteToFile struct {
	header http.Header
	io.Writer
}

func (w *WriteToFile) Header() http.Header {
	// return w.w.Header()
	return w.header
}

//	func (w *WriteToFile) Write(p []byte) (int, error) {
//		// w.w.Write(p)
//		return w.Writer.Write(p)
//	}
func (w *WriteToFile) WriteHeader(statusCode int) {
	// w.w.WriteHeader(statusCode)
}

func (p *DebugPlugin) OnInit() error {
	if p.Profile != "" {
		go func() {
			file, err := os.Create(p.Profile)
			if err != nil {
				return
			}
			defer file.Close()
			p.Info("cpu profile start")
			err = runtimePPROF.StartCPUProfile(file)
			time.Sleep(p.ProfileDuration)
			runtimePPROF.StopCPUProfile()
			p.Info("cpu profile done")
		}()
	}
	return nil
}

func (p *DebugPlugin) Pprof_Trace(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/debug" + r.URL.Path
	pprof.Trace(w, r)
}

func (p *DebugPlugin) Pprof_profile(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/debug" + r.URL.Path
	pprof.Profile(w, r)
}

func (p *DebugPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/pprof" {
		http.Redirect(w, r, "/debug/pprof/", http.StatusFound)
		return
	}
	r.URL.Path = "/debug" + r.URL.Path
	pprof.Index(w, r)
}

func (p *DebugPlugin) Charts_(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/static" + strings.TrimPrefix(r.URL.Path, "/charts")
	staticFSHandler.ServeHTTP(w, r)
}

func (p *DebugPlugin) Charts_data(w http.ResponseWriter, r *http.Request) {
	dataHandler(w, r)
}

func (p *DebugPlugin) Charts_datafeed(w http.ResponseWriter, r *http.Request) {
	s.dataFeedHandler(w, r)
}

func (p *DebugPlugin) Grf(w http.ResponseWriter, r *http.Request) {
	dConf := debugger.Config{
		AttachPid:             os.Getpid(),
		Backend:               "default",
		CoreFile:              "",
		DebugInfoDirectories:  conf.DebugInfoDirectories,
		AttachWaitFor:         "",
		AttachWaitForInterval: 1,
		AttachWaitForDuration: 0,
	}
	dbg, err := debugger.New(&dConf, nil)
	defer dbg.Detach(false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = myproc.ObjectReference(dbg.Target(), p.Grfout); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Write([]byte("ok"))
}
