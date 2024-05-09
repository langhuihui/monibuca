//go:build linux && !darwin

package util

import (
	"os"
	"syscall"
)

func init() {
	logFile := initFatalLog()
	if logFile != nil {
		// 将进程标准出错重定向至文件，进程崩溃时运行时将向该文件记录协程调用栈信息
		syscall.Dup3(int(logFile.Fd()), int(os.Stderr.Fd()), 0)
	}
}
