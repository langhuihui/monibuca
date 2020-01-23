package monica

import (
	"io"
	"log"
)

type LogWriter struct {
	io.Writer
	origin io.Writer
}

func (w *LogWriter) Write(data []byte) (n int, err error) {
	if n, err = w.Writer.Write(data); err != nil {
		go log.SetOutput(w.origin)
	}
	return w.origin.Write(data)
}

func AddWriter(wn io.Writer) {
	log.SetOutput(&LogWriter{
		Writer: wn,
		origin: log.Writer(),
	})
}

func MayBeError(info error) (hasError bool) {
	if hasError = info != nil; hasError {
		log.Print(info)
	}
	return
}
