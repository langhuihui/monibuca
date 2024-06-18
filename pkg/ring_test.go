package pkg

import (
	"context"
	"testing"
	"time"
)

func TestRing(t *testing.T) {
	w := NewRingWriter(10)
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	go t.Run("writer", func(t *testing.T) {
		for i := 0; ctx.Err() == nil; i++ {
			w.Value.Raw = i
			normal := w.Step()
			t.Log("write", i, normal)
			time.Sleep(time.Millisecond * 50)
		}
		w.Value.Unlock()
	})
	go t.Run("reader1", func(t *testing.T) {
		var reader RingReader
		err := reader.StartRead(w.Ring)
		if err != nil {
			t.Error(err)
			return
		}
		for ctx.Err() == nil {
			err = reader.ReadNext()
			t.Log("read1", reader.Value.Raw)
			if err != nil {
				t.Error(err)
				break
			}
			time.Sleep(time.Millisecond * 10)
		}
		reader.StopRead()
		<-ctx.Done()
	})
	// slow reader
	t.Run("reader2", func(t *testing.T) {
		var reader RingReader
		err := reader.StartRead(w.Ring)
		if err != nil {
			t.Error(err)
			return
		}
		for ctx.Err() == nil {
			err = reader.ReadNext()
			t.Log("read2", reader.Value.Raw)
			if err != nil {
				// t.Error(err)
				return
			}
			time.Sleep(time.Millisecond * 100)
		}
		reader.StopRead()
		<-ctx.Done()
	})
}
func TestRingWriter_Resize(t *testing.T) {
	w := NewRingWriter(10)
	w.Resize(5)
	w.Resize(-5)
	w.Resize(5)
	w.Resize(-5)
}
func BenchmarkRing(b *testing.B) {
	w := NewRingWriter(10)
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	go func() {
		for i := 0; ctx.Err() == nil; i++ {
			w.Value.Raw = i
			w.Step()
			time.Sleep(time.Millisecond * 50)
		}
		w.Value.Unlock()
	}()
	b.SetParallelism(1000)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var reader RingReader
			err := reader.StartRead(w.Ring)
			if err != nil {
				break
			}
			for ctx.Err() == nil {
				err = reader.ReadNext()
				if err != nil {
					break
				}
				time.Sleep(time.Millisecond * 10)
			}
			reader.Value.RUnlock()
		}
	})
}
