package plugin_console

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
	"m7s.live/m7s/v5"
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
	Server string `default:"console.monibuca.com:44944" desc:"远程控制台地址"` //远程控制台地址
	Secret string `desc:"远程控制台密钥"`                                      //远程控制台密钥
}

var _ = m7s.InstallPlugin[ConsolePlugin]()

func (cfg *ConsolePlugin) OnInit() error {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"monibuca"},
	}
	conn, err := quic.DialAddr(cfg.Context, cfg.Server, tlsConf, &quic.Config{
		KeepAlivePeriod: time.Second * 10,
		EnableDatagrams: true,
	})
	if stream := quic.Stream(nil); err == nil {
		if stream, err = conn.OpenStreamSync(cfg.Context); err == nil {
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
	go func() {
		for err == nil {
			var s quic.Stream
			if s, err = conn.AcceptStream(cfg.Context); err == nil {
				go cfg.ReceiveRequest(s, conn)
			}
		}
	}()
	return err
}

func (cfg *ConsolePlugin) ReceiveRequest(s quic.Stream, conn quic.Connection) error {
	defer s.Close()
	wr := &myResponseWriter2{Stream: s}
	reader := bufio.NewReader(s)
	var req *http.Request
	url, _, err := reader.ReadLine()
	if err == nil {
		ctx, cancel := context.WithCancel(s.Context())
		defer cancel()
		req, err = http.NewRequestWithContext(ctx, "GET", string(url), reader)
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

		if err == nil {
			h := cfg.GetGlobalCommonConf().GetHandler()
			if req.Header.Get("Accept") == "text/event-stream" {
				go h.ServeHTTP(wr, req)
			} else if req.Header.Get("Upgrade") == "websocket" {
				var writer myResponseWriter3
				writer.Stream = s
				writer.Connection = conn
				req.Host = req.Header.Get("Host")
				if req.Host == "" {
					req.Host = req.URL.Host
				}
				if req.Host == "" {
					req.Host = "m7s.live"
				}
				h.ServeHTTP(&writer, req) //建立websocket连接,握手
			} else {
				h.ServeHTTP(wr, req)
			}
		}
		io.ReadAll(s)
	}
	if err != nil {
		cfg.Error("read console server", "err", err)
	}
	return err
}
