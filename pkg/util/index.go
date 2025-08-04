package util

import (
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

type ReadWriteSeekCloser interface {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer
}

type Recyclable interface {
	Recycle()
}

type Object = map[string]any

func Conditional[T any](cond bool, t, f T) T {
	if cond {
		return t
	} else {
		return f
	}
}

// Bit1 检查字节中的某一位是否为1 |0|1|2|3|4|5|6|7|
func Bit1(b byte, index int) bool {
	return b&(1<<(7-index)) != 0
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func RandomString(length int) string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func RandomNumString(length int) string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = "0123456789"[seededRand.Intn(10)]
	}
	return string(b)
}

func InitFatalLog(fatal_log_dir string) *os.File {
	os.MkdirAll(fatal_log_dir, 0766)
	fatal_log := filepath.Join(fatal_log_dir, "latest.log")
	info, err := os.Stat(fatal_log)
	if err == nil && info.Size() != 0 {
		os.Rename(fatal_log, filepath.Join(fatal_log_dir, info.ModTime().Format("2006-01-02 15:04:05")+".log"))
	}
	logFile, err := os.OpenFile(fatal_log, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		log.Println("服务启动出错", "打开异常日志文件失败", err)
		return nil
	}
	return logFile
}

func Exist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}

type ReuseArray[T any] []T

func (s *ReuseArray[T]) GetNextPointer() (r *T) {
	ss := *s
	l := len(ss)
	if cap(ss) > l {
		ss = ss[:l+1]
	} else {
		var new T
		ss = append(ss, new)
	}
	*s = ss
	r = &((ss)[l])
	if resetter, ok := any(r).(Resetter); ok {
		resetter.Reset()
	}
	return r
}

func (s ReuseArray[T]) RangePoint(f func(yield *T) bool) {
	for i := range len(s) {
		if !f(&s[i]) {
			return
		}
	}
}

func (s *ReuseArray[T]) Reset() {
	*s = (*s)[:0]
}

func (s *ReuseArray[T]) Reduce() ReuseArray[T] {
	ss := *s
	ss = ss[:len(ss)-1]
	*s = ss
	return ss
}

func (s *ReuseArray[T]) Remove(item *T) bool {
	for i := range *s {
		if &(*s)[i] == item {
			*s = append((*s)[:i], (*s)[i+1:]...)
			return true
		}
	}
	return false
}

func (s *ReuseArray[T]) Count() int {
	return len(*s)
}

type Resetter interface {
	Reset()
}
