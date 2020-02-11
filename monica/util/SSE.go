package util

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
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
	_, err = sse.ResponseWriter.Write(sseBegin)
	n, err = sse.ResponseWriter.Write(data)
	_, err = sse.ResponseWriter.Write(sseEnd)
	if err != nil {
		return
	}
	sse.ResponseWriter.(http.Flusher).Flush()
	return
}

func (sse *SSE) WriteEvent(event string, data []byte) (err error) {
	if err = sse.Err(); err != nil {
		return
	}
	_, err = sse.ResponseWriter.Write(sseEent)
	_, err = sse.ResponseWriter.Write([]byte(event))
	_, err = sse.ResponseWriter.Write([]byte("\n"))
	_, err = sse.Write(data)
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
