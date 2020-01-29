package gateway

import (
	"context"
	"encoding/json"
	. "github.com/langhuihui/monibuca/monica"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os/exec"
	"path"
	"runtime"
	"time"
)

var (
	config        = new(ListenerConfig)
	sseBegin      = []byte("data: ")
	sseEnd        = []byte("\n\n")
	dashboardPath string
)

type SSE struct {
	http.ResponseWriter
	context.Context
}

func (sse *SSE) Write(data []byte) (n int, err error) {
	if err = sse.Err(); err != nil {
		return
	}
	_, err = sse.ResponseWriter.Write(sseBegin)
	n, err = sse.ResponseWriter.Write(data)
	_, err = sse.ResponseWriter.Write(sseEnd)
	if err != nil {
		return
	}
	sse.ResponseWriter.(http.Flusher).Flush()
	return
}
func NewSSE(w http.ResponseWriter, ctx context.Context) *SSE {
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	header.Set("Access-Control-Allow-Origin", "*")
	return &SSE{
		w,
		ctx,
	}
}

func (sse *SSE) WriteJSON(data interface{}) (err error) {
	var jsonData []byte
	if jsonData, err = json.Marshal(data); err == nil {
		if _, err = sse.Write(jsonData); err != nil {
			return
		}
		return
	}
	return
}
func (sse *SSE) WriteExec(cmd *exec.Cmd) error {
	cmd.Stderr = sse
	cmd.Stdout = sse
	return cmd.Run()
}

func init() {
	_, currentFilePath, _, _ := runtime.Caller(0)
	dashboardPath = path.Join(path.Dir(currentFilePath), "../../dashboard/dist")
	log.Println(dashboardPath)
	InstallPlugin(&PluginConfig{
		Name:   "GateWay",
		Type:   PLUGIN_HOOK,
		Config: config,
		Run:    run,
	})
}
func run() {
	http.HandleFunc("/api/summary", summary)
	http.HandleFunc("/", website)
	log.Printf("server gateway start at %s", config.ListenAddr)
	log.Fatal(http.ListenAndServe(config.ListenAddr, nil))
}
func website(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Path
	if filePath == "/" {
		filePath = "/index.html"
	} else if filePath == "/docs" {
		filePath = "/docs/index.html"
	}
	if mime := mime.TypeByExtension(path.Ext(filePath)); mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	if f, err := ioutil.ReadFile(dashboardPath + filePath); err == nil {
		if _, err = w.Write(f); err != nil {
			w.WriteHeader(505)
		}
	} else {
		w.Header().Set("Location", "/")
		w.WriteHeader(302)
	}
}
func summary(w http.ResponseWriter, r *http.Request) {
	sse := NewSSE(w, r.Context())
	s := collect()
	sse.WriteJSON(&s)
	for range time.NewTicker(time.Second).C {
		old := s
		s = collect()
		for i, v := range s.NetWork {
			s.NetWork[i].ReceiveSpeed = v.Receive - old.NetWork[i].Receive
			s.NetWork[i].SentSpeed = v.Sent - old.NetWork[i].Sent
		}
		AllRoom.Range(func(key interface{}, v interface{}) bool {
			s.Rooms = append(s.Rooms, &v.(*Room).RoomInfo)
			return true
		})
		if sse.WriteJSON(&s) != nil {
			break
		}
	}
}

type Summary struct {
	Memory struct {
		Total uint64
		Free  uint64
		Used  uint64
		Usage float64
	}
	CPUUsage float64
	HardDisk struct {
		Total uint64
		Free  uint64
		Used  uint64
		Usage float64
	}
	NetWork []NetWorkInfo
	Rooms   []*RoomInfo
}
type NetWorkInfo struct {
	Name         string
	Receive      uint64
	Sent         uint64
	ReceiveSpeed uint64
	SentSpeed    uint64
}

func collect() (s Summary) {
	v, _ := mem.VirtualMemory()
	//c, _ := cpu.Info()
	cc, _ := cpu.Percent(time.Second, false)
	d, _ := disk.Usage("/")
	//n, _ := host.Info()
	nv, _ := net.IOCounters(true)
	//boottime, _ := host.BootTime()
	//btime := time.Unix(int64(boottime), 0).Format("2006-01-02 15:04:05")
	s.Memory.Total = v.Total / 1024 / 1024
	s.Memory.Free = v.Available / 1024 / 1024
	s.Memory.Used = v.Used / 1024 / 1024
	s.Memory.Usage = v.UsedPercent
	//fmt.Printf("        Mem       : %v MB  Free: %v MB Used:%v Usage:%f%%\n", v.Total/1024/1024, v.Available/1024/1024, v.Used/1024/1024, v.UsedPercent)
	//if len(c) > 1 {
	//	for _, sub_cpu := range c {
	//		modelname := sub_cpu.ModelName
	//		cores := sub_cpu.Cores
	//		fmt.Printf("        CPU       : %v   %v cores \n", modelname, cores)
	//	}
	//} else {
	//	sub_cpu := c[0]
	//	modelname := sub_cpu.ModelName
	//	cores := sub_cpu.Cores
	//	fmt.Printf("        CPU       : %v   %v cores \n", modelname, cores)
	//}
	s.CPUUsage = cc[0]
	s.HardDisk.Free = d.Free / 1024 / 1024 / 1024
	s.HardDisk.Total = d.Total / 1024 / 1024 / 1024
	s.HardDisk.Used = d.Used / 1024 / 1024 / 1024
	s.HardDisk.Usage = d.UsedPercent
	s.NetWork = make([]NetWorkInfo, len(nv))
	for i, n := range nv {
		s.NetWork[i].Name = n.Name
		s.NetWork[i].Receive = n.BytesRecv
		s.NetWork[i].Sent = n.BytesSent
	}

	//fmt.Printf("        Network: %v bytes / %v bytes\n", nv[0].BytesRecv, nv[0].BytesSent)
	//fmt.Printf("        SystemBoot:%v\n", btime)
	//fmt.Printf("        CPU Used    : used %f%% \n", cc[0])
	//fmt.Printf("        HD        : %v GB  Free: %v GB Usage:%f%%\n", d.Total/1024/1024/1024, d.Free/1024/1024/1024, d.UsedPercent)
	//fmt.Printf("        OS        : %v(%v)   %v  \n", n.Platform, n.PlatformFamily, n.PlatformVersion)
	//fmt.Printf("        Hostname  : %v  \n", n.Hostname)
	return
}
