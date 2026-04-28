//go:build windows && systray
// +build windows,systray

package systray

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/getlantern/systray"
)

var (
	// systrayQuit 用于通知主程序退出
	systrayQuit chan struct{}
	// serverContext 用于取消服务器上下文
	serverContext context.Context
	serverCancel  context.CancelFunc
	// execDir 程序执行目录
	execDir string
)

func init() {
	// 禁用彩色输出，避免日志文件出现 ANSI 控制字符
	_ = os.Setenv("NO_COLOR", "1")
	_ = os.Setenv("CLICOLOR", "0")
}

// StartSystray 启动系统托盘（仅在 Windows 且启用 systray tag 时）
// execDirPath 是程序执行目录路径，用于查找图标文件
func StartSystray(ctx context.Context, cancel context.CancelFunc, execDirPath string) {
	serverContext = ctx
	serverCancel = cancel
	execDir = execDirPath
	systrayQuit = make(chan struct{})

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
