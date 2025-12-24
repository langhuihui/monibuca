//go:build windows && systray
// +build windows,systray

package systray

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows"
)

var (
	// systrayQuit 用于通知主程序退出
	systrayQuit chan struct{}
	// serverContext 用于取消服务器上下文
	serverContext context.Context
	serverCancel  context.CancelFunc
	// execDir 程序执行目录
	execDir string
	// 确保重定向只执行一次
	redirectOnce sync.Once
	// ANSI 转义序列清理
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]|\x1b\][^\a]*\a`)
	// 日志文件写锁
	logMu sync.Mutex
)

func init() {
	// 禁用彩色输出，避免日志文件出现 ANSI 控制字符
	_ = os.Setenv("NO_COLOR", "1")
	_ = os.Setenv("CLICOLOR", "0")

	// 尝试在包初始化时就重定向，防止早期日志写入无效句柄
	if exe, err := os.Executable(); err == nil {
		redirectStdIO(filepath.Dir(exe))
	}
}

// StartSystray 启动系统托盘（仅在 Windows 且启用 systray tag 时）
// execDirPath 是程序执行目录路径，用于查找图标文件
func StartSystray(ctx context.Context, cancel context.CancelFunc, execDirPath string) {
	serverContext = ctx
	serverCancel = cancel
	execDir = execDirPath
	systrayQuit = make(chan struct{})

	// 在托盘启动前将 stdout/stderr 重定向到文件，避免 GUI 模式下看不到日志
	redirectStdIO(execDir)

	// 在 goroutine 中运行托盘
	go systray.Run(onReady, onExit)
}

// onReady 托盘准备就绪时的回调
func onReady() {
	// 设置托盘图标（使用默认图标数据，可以后续替换为实际图标）
	systray.SetIcon(getIcon())
	systray.SetTitle("M7S 流媒体服务器")
	systray.SetTooltip("M7S 流媒体服务器正在运行")

	// 添加退出菜单项
	mQuit := systray.AddMenuItem("退出", "退出程序")

	// 监听退出菜单点击
	go func() {
		<-mQuit.ClickedCh
		// 通知主程序退出
		close(systrayQuit)
		// 取消服务器上下文，触发优雅关闭
		if serverCancel != nil {
			serverCancel()
		}
		systray.Quit()
		// 兜底强退，防止部分任务阻塞不退出
		time.AfterFunc(3*time.Second, func() {
			os.Exit(0)
		})
	}()
}

// onExit 托盘退出时的清理回调
func onExit() {
	// 清理工作可以在这里进行
}

// getIcon 返回托盘图标数据
// 优先使用 NVR.ico，其次 favicon.ico，最后使用默认图标
func getIcon() []byte {
	// 默认图标数据（简单的 16x16 ICO 格式图标）
	iconData := []byte{
		0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10, 0x00, 0x00, 0x01, 0x00,
		0x20, 0x00, 0x68, 0x04, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00, 0x28, 0x00,
		0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x01, 0x00,
		0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
	}

	if execDir == "" {
		return iconData
	}

	// 优先尝试读取 NVR.ico 文件
	iconPath := filepath.Join(execDir, "NVR.ico")
	if data, err := os.ReadFile(iconPath); err == nil && len(data) > 0 {
		return data
	}

	// 其次尝试读取 favicon.ico 文件
	iconPath = filepath.Join(execDir, "favicon.ico")
	if data, err := os.ReadFile(iconPath); err == nil && len(data) > 0 {
		return data
	}

	// 如果都读取失败，返回默认图标
	return iconData
}

// WaitForSystrayQuit 等待托盘退出信号
func WaitForSystrayQuit() <-chan struct{} {
	if systrayQuit == nil {
		// 如果未启用托盘，创建一个永远不会发送信号的 channel
		ch := make(chan struct{})
		return ch
	}
	return systrayQuit
}

// redirectStdIO 将标准输出/错误重定向到 logs/m7s.log（追加写入）
func redirectStdIO(execDir string) {
	redirectOnce.Do(func() {
		if execDir == "" {
			return
		}
		logDir := filepath.Join(execDir, "logs")
		logFile := filepath.Join(logDir, "m7s.log")

		// 确保日志目录存在
		_ = os.MkdirAll(logDir, 0o755)

		// 追加方式打开日志文件
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}

		// 启动 stdout/stderr 管道，去除 ANSI 颜色后写入同一个日志文件
		startPipeRedirect(f, windows.STD_OUTPUT_HANDLE, "/dev/stdout")
		startPipeRedirect(f, windows.STD_ERROR_HANDLE, "/dev/stderr")

		// slog 默认使用 log 包输出错误，保持一致
		log.SetOutput(f)
	})
}

// startPipeRedirect 创建管道，将句柄重定向到管道写端，读端去除 ANSI 后写入日志文件
func startPipeRedirect(logFile *os.File, stdHandle uint32, name string) {
	r, w, err := os.Pipe()
	if err != nil {
		return
	}

	// 设置底层句柄为管道写端
	_ = windows.SetStdHandle(stdHandle, windows.Handle(w.Fd()))

	// 更新 os.Stdout/os.Stderr 指向管道写端
	if stdHandle == windows.STD_OUTPUT_HANDLE {
		os.Stdout = w
	} else if stdHandle == windows.STD_ERROR_HANDLE {
		os.Stderr = w
	}

	// 后台 goroutine 读管道，清理 ANSI，再写日志文件
	go func() {
		defer r.Close()
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				clean := ansiRe.ReplaceAll(buf[:n], []byte{})
				logMu.Lock()
				_, _ = logFile.Write(clean)
				logMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
}
