package cascade

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/quic-go/quic-go"
	"m7s.live/v5/pkg/task"
)

type Http2Quic struct {
	task.Task
	quic.Connection
	quic.Stream
}

func (q *Http2Quic) ServeSSE(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	header.Set("Access-Control-Allow-Origin", "*")
	b := make([]byte, 1024)
	q.Read(b[:2])
	defer q.Close()
	for r.Context().Err() == nil {
		if n, err := q.Read(b); err == nil {
			if _, err = w.Write(b[:n]); err == nil {
				w.(http.Flusher).Flush()
			} else {
				return
			}
		} else {
			return
		}
	}
}

func (q *Http2Quic) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	_, rw, _, err := ws.UpgradeHTTP(r, w)
	// b := make([]byte, 1024)
	defer q.Close()
	for {
		// 过滤掉websocket握手的http头部
		l, _, _ := rw.ReadLine()
		if len(l) == 0 {
			break
		}
	}
	go func() {
		var msg []byte
		var err error
		for err == nil {
			msg, err = wsutil.ReadServerText(rw)
			q.Debug("read server", "msg", string(msg))
			if err == nil {
				err = wsutil.WriteServerText(rw, msg)
			}
		}
	}()
	var msg []byte
	for err == nil {
		msg, err = wsutil.ReadClientText(rw)
		q.Debug("read client", "msg", string(msg))
		if err == nil {
			err = wsutil.WriteClientText(rw, msg)
		}
	}
	if err != nil {
		q.Error("websocket", "err", err)
	}
}

// 必须先调用 Request 建立通道才能将 Http 请求转换成 quic 请求
func (q *Http2Quic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	relayURL := r.URL.Path
	if r.URL.RawQuery != "" {
		relayURL += "?" + r.URL.RawQuery
	}
	fmt.Fprintf(q, "%s %s\r\n", r.Method, relayURL)
	r.Header.Write(q)
	fmt.Fprint(q, "\r\n")
	if r.Header.Get("Accept") == "text/event-stream" {
		q.ServeSSE(w, r)
	} else if r.Header.Get("Upgrade") == "websocket" {
		q.ServeWebSocket(w, r)
	} else {
		io.Copy(q, r.Body)
		q.Close()
		io.Copy(w, q)
	}
}
