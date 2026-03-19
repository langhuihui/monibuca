package cascade

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	task "github.com/langhuihui/gotask"
	flv "m7s.live/v5/plugin/flv/pkg"

	"github.com/quic-go/quic-go"
	"m7s.live/v5"
)

type RelayAPIConfig struct {
	Allow []string `desc:"允许转发的路径前缀"` //允许转发的路径
	Deny  []string `desc:"禁止转发的路径前缀"` //禁止转发的路径
}

func (c *RelayAPIConfig) Check(path string) bool {
	if len(c.Allow) > 0 {
		for _, p := range c.Allow {
			if strings.HasPrefix(path, p) {
				return true
			}
		}
		return false
	}
	if len(c.Deny) > 0 {
		for _, p := range c.Deny {
			if strings.HasPrefix(path, p) {
				return false
			}
		}
		return true
	}
	return true
}

type ReceiveRequestTask struct {
	task.Task
	Plugin *m7s.Plugin
	*quic.Conn
	*quic.Stream
	http.Handler
	*RelayAPIConfig
}

func (task *ReceiveRequestTask) Go() (err error) {
	reader := bufio.NewReader(task.Stream)
	line0, _, err := reader.ReadLine()
	reqLine := strings.Split(string(line0), " ")
	var req *http.Request
	task.SetDescription("request", line0)
	if err == nil {
		ctx, cancel := context.WithCancel(task.Stream.Context())
		defer cancel()
		switch reqLine[0] {
		case "PULLFLV":
			var live flv.Live
			if live.Subscriber, err = task.Plugin.Subscribe(task.Task.Context, reqLine[1]); err == nil {
				live.WriteFlvTag = func(tag net.Buffers) (err error) {
					_, err = tag.WriteTo(task.Stream)
					return err
				}
			}
			// 保存 subscriber 引用，用于手动停止
			subscriber := live.Subscriber
			// 启动 goroutine 监听停止命令
			go func() {
				stopReader := bufio.NewReader(task.Stream)
				for {
					line, _, err := stopReader.ReadLine()
					if err != nil {
						break
					}
					if strings.HasPrefix(string(line), "STOPFLV") {
						fmt.Printf("[cascade] 收到 STOPFLV: %s\n", line)
						// 直接调用 subscriber.Stop() 停止订阅者
						subscriber.Stop(errors.New("stop by STOPFLV command"))
						cancel()
						break
					}
				}
			}()
			return live.Run()
		}
		req, err = http.NewRequestWithContext(ctx, reqLine[0], reqLine[1], reader)
		for err == nil {
			var h []byte
			if h, _, err = reader.ReadLine(); len(h) > 0 {
				if b, a, f := strings.Cut(string(h), ": "); f {
					req.Header.Set(b, a)
				}
			} else {
				break
			}
		}
	}
	if err != nil {
		return
	}
	//h, pattern := task.handler.Handler(req)
	//if !task.Check(pattern) {
	//	http.Error(task, "403 Forbidden", http.StatusForbidden)
	//	return err
	//}
	if req.Header.Get("Accept") == "text/event-stream" {
		go task.ServeHTTP(task, req)
	} else if req.Header.Get("Upgrade") == "websocket" {
		req.Host = req.Header.Get("Host")
		if req.Host == "" {
			req.Host = req.URL.Host
		}
		if req.Host == "" {
			req.Host = "m7s.live"
		}
		task.ServeHTTP(task, req) //建立websocket连接,握手
	} else {
		method := req.Header.Get("M7s-Method")
		if method == "POST" {
			req.Method = "POST"
		}
		task.ServeHTTP(task, req)
	}
	_, err = io.ReadAll(task)
	return
}

func (task *ReceiveRequestTask) Dispose() {
	task.Stream.Close()
}

func (q *ReceiveRequestTask) Header() http.Header {
	return make(http.Header)
}
func (q *ReceiveRequestTask) WriteHeader(statusCode int) {

}
func (q *ReceiveRequestTask) Flush() {
}

func (q *ReceiveRequestTask) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return net.Conn(q), bufio.NewReadWriter(bufio.NewReader(q), bufio.NewWriter(q)), nil
}
