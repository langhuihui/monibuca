package util

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os/exec"

	"gopkg.in/yaml.v3"
)

var (
	sseEent  = []byte("event: ")
	sseBegin = []byte("data: ")
	sseEnd   = []byte("\n\n")
)

type SSE struct {
	http.ResponseWriter
	context.Context
}

func (sse *SSE) Write(data []byte) (n int, err error) {
	if err = sse.Err(); err != nil {
		return
	}
	buffers := net.Buffers{sseBegin, data, sseEnd}
	nn, err := buffers.WriteTo(sse.ResponseWriter)
	if err == nil {
		sse.ResponseWriter.(http.Flusher).Flush()
	}
	return int(nn), err
}

func (sse *SSE) WriteEvent(event string, data []byte) (err error) {
	if err = sse.Err(); err != nil {
		return
	}
	buffers := net.Buffers{sseEent, []byte(event + "\n"), sseBegin, data, sseEnd}
	_, err = buffers.WriteTo(sse.ResponseWriter)
	if err == nil {
		sse.ResponseWriter.(http.Flusher).Flush()
	}
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
		ResponseWriter: w,
		Context:        ctx,
	}
}

func (sse *SSE) WriteJSON(data any) error {
	return json.NewEncoder(sse).Encode(data)
}
func (sse *SSE) WriteYAML(data any) error {
	return yaml.NewEncoder(sse).Encode(data)
}
func (sse *SSE) WriteExec(cmd *exec.Cmd) error {
	cmd.Stderr = sse
	cmd.Stdout = sse
	return cmd.Run()
}
