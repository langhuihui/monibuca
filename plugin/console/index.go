package plugin_console

import (
	"bufio"
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/quic-go/quic-go"
	"io"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type myResponseWriter struct {
}

func (*myResponseWriter) Header() http.Header {
	return make(http.Header)
}
func (*myResponseWriter) WriteHeader(statusCode int) {
}
func (w *myResponseWriter) Flush() {
}

type myResponseWriter2 struct {
	quic.Stream
	myResponseWriter
}

type myResponseWriter3 struct {
	handshake bool
	myResponseWriter2
	quic.Connection
}

func (w *myResponseWriter3) Write(b []byte) (int, error) {
	if !w.handshake {
		w.handshake = true
		return len(b), nil
	}
	println(string(b))
	return w.Stream.Write(b)
}

func (w *myResponseWriter3) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return net.Conn(w), bufio.NewReadWriter(bufio.NewReader(w), bufio.NewWriter(w)), nil
}

type ConsolePlugin struct {
	m7s.Plugin
	Server string `default:"console.monibuca.com:17173" desc:"远程控制台地址"` //远程控制台地址
	Secret string `desc:"远程控制台密钥"`                                      //远程控制台密钥
}

var _ = m7s.InstallPlugin[ConsolePlugin]()

type ConnectServerTask struct {
	task.Task
	cfg *ConsolePlugin
	quic.Connection
}

func (task *ConnectServerTask) Start() (err error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"monibuca"},
	}
	cfg := task.cfg
	task.Connection, err = quic.DialAddr(cfg.Context, cfg.Server, tlsConf, &quic.Config{
		KeepAlivePeriod: time.Second * 10,
		EnableDatagrams: true,
	})
	if stream := quic.Stream(nil); err == nil {
		if stream, err = task.OpenStreamSync(cfg.Context); err == nil {
			_, err = stream.Write(append([]byte{1}, (cfg.Secret + "\n")...))
			if msg := []byte(nil); err == nil {
				if msg, err = bufio.NewReader(stream).ReadSlice(0); err == nil {
					var rMessage map[string]any
					if err = json.Unmarshal(msg[:len(msg)-1], &rMessage); err == nil {
						if rMessage["code"].(float64) != 0 {
							// cfg.Error("response from console server ", cfg.Server, rMessage["msg"])
							return fmt.Errorf("response from console server %s %s", cfg.Server, rMessage["msg"])
						} else {
							// cfg.reportStream = stream
							cfg.Info("response from console server ", cfg.Server, rMessage)
							// if v, ok := rMessage["enableReport"]; ok {
							// 	cfg.enableReport = v.(bool)
							// }
							// if v, ok := rMessage["instanceId"]; ok {
							// 	cfg.instanceId = v.(string)
							// }
						}
					}
				}
			}
		}
	}
	return
}

func (task *ConnectServerTask) Run() (err error) {
	for err == nil {
		var s quic.Stream
		if s, err = task.AcceptStream(task.Task.Context); err == nil {
			task.cfg.AddTask(&ReceiveRequestTask{
				stream:  s,
				handler: task.cfg.GetGlobalCommonConf().GetHandler(),
				conn:    task.Connection,
			})
		}
	}
	return
}

type ReceiveRequestTask struct {
	task.Task
	stream  quic.Stream
	handler http.Handler
	conn    quic.Connection
	req     *http.Request
}

func (task *ReceiveRequestTask) Start() (err error) {
	reader := bufio.NewReader(task.stream)
	url, _, err := reader.ReadLine()
	if err == nil {
		ctx, cancel := context.WithCancel(task.stream.Context())
		defer cancel()
		task.req, err = http.NewRequestWithContext(ctx, "GET", string(url), reader)
		for err == nil {
			var h []byte
			if h, _, err = reader.ReadLine(); len(h) > 0 {
				if b, a, f := strings.Cut(string(h), ": "); f {
					task.req.Header.Set(b, a)
				}
			} else {
				break
			}
		}
	}
	return
}

func (task *ReceiveRequestTask) Run() (err error) {
	wr := &myResponseWriter2{Stream: task.stream}
	req := task.req
	if req.Header.Get("Accept") == "text/event-stream" {
		go task.handler.ServeHTTP(wr, req)
	} else if req.Header.Get("Upgrade") == "websocket" {
		var writer myResponseWriter3
		writer.Stream = task.stream
		writer.Connection = task.conn
		req.Host = req.Header.Get("Host")
		if req.Host == "" {
			req.Host = req.URL.Host
		}
		if req.Host == "" {
			req.Host = "m7s.live"
		}
		task.handler.ServeHTTP(&writer, req) //建立websocket连接,握手
	} else {
		method := req.Header.Get("M7s-Method")
		if method == "POST" {
			req.Method = "POST"
		}
		task.handler.ServeHTTP(wr, req)
	}
	_, err = io.ReadAll(task.stream)
	return
}

func (task *ReceiveRequestTask) Dispose() {
	task.stream.Close()
}

func (cfg *ConsolePlugin) OnInit() error {
	if cfg.Secret == "" || cfg.Server == "" {
		return nil
	}
	connectTask := ConnectServerTask{
		cfg: cfg,
	}
	connectTask.SetRetry(-1, time.Second)
	cfg.AddTask(&connectTask)
	return nil
}

//go:embed web/*
var uiFiles embed.FS
var fileServer = http.FileServer(http.FS(uiFiles))

func (cfg *ConsolePlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	embedPath := "/web" + r.URL.Path
	if r.URL.Path == "/" {
		r.URL.Path = "/web/index.html"
	} else {
		r.URL.Path = "/web" + r.URL.Path
	}
	file, err := os.Open("./" + r.URL.Path)
	if err == nil {
		defer file.Close()
		http.ServeContent(w, r, r.URL.Path, time.Now(), file)
		return
	}
	r.URL.Path = embedPath
	fileServer.ServeHTTP(w, r)
}
