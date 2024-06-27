package util

import (
	"log"
	"os"
	"path/filepath"
)

func Conditoinal[T any](cond bool, t, f T) T {
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

func LenOfBuffers(b [][]byte) (n int) {
	for _, bb := range b {
		n += len(bb)
	}
	return
}

func initFatalLog() *os.File {
	fatal_log_dir := "./fatal"
	if _fatal_log := os.Getenv("M7S_FATAL_LOG"); _fatal_log != "" {
		fatal_log_dir = _fatal_log
	}
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