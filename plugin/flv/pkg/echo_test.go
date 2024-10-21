package flv

import (
	"errors"
	"io"
	"m7s.live/v5/pkg/util"
	"net"
	"os"
	"testing"
)

func TestRead(t *testing.T) {
	var feeder = make(chan net.Buffers, 100)
	reader := util.NewBufReaderBuffersChan(feeder)

	t.Run("feed", func(t *testing.T) {
		t.Parallel()
		file, _ := os.Open("/Users/dexter/Downloads/ps.flv")
		for {
			var buf = make([]byte, 1024)
			n, err := file.Read(buf)
			if err != nil {
				close(feeder)
				break
			}
			feeder <- net.Buffers{buf[:n]}
		}
	})
	t.Run("read", func(t *testing.T) {
		t.Parallel()
		err := Echo(reader)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Error(err)
			t.FailNow()
		}
	})
}
