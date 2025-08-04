package util

import (
	"io"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws/wsutil"
)

type HTTP_WS_Writer struct {
	io.Writer
	Conn         net.Conn
	ContentType  string
	WriteTimeout time.Duration
	IsWebSocket  bool
	buffer       []byte
}

func (m *HTTP_WS_Writer) Write(p []byte) (n int, err error) {
	if m.IsWebSocket {
		m.buffer = append(m.buffer, p...)
		return len(p), nil
	}
	if m.Conn != nil && m.WriteTimeout > 0 {
		m.Conn.SetWriteDeadline(time.Now().Add(m.WriteTimeout))
	}
	return m.Writer.Write(p)
}

func (m *HTTP_WS_Writer) Flush() (err error) {
	if m.IsWebSocket {
		if m.WriteTimeout > 0 {
			m.Conn.SetWriteDeadline(time.Now().Add(m.WriteTimeout))
		}
		err = wsutil.WriteServerBinary(m.Conn, m.buffer)
		m.buffer = m.buffer[:0]
	}
	return
}

func (m *HTTP_WS_Writer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.Conn == nil {
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Content-Type", m.ContentType)
		w.WriteHeader(http.StatusOK)
		if hijacker, ok := w.(http.Hijacker); ok && m.WriteTimeout > 0 {
			m.Conn, _, _ = hijacker.Hijack()
			m.Conn.SetWriteDeadline(time.Now().Add(m.WriteTimeout))
			m.Writer = m.Conn
		} else {
			m.Writer = w
			w.(http.Flusher).Flush()
		}
	} else {
		m.IsWebSocket = true
		m.Writer = m.Conn
	}
}
