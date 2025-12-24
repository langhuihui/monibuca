//go:build !windows || !systray
// +build !windows !systray

package systray

import "context"

// StartSystray 空实现（非 Windows 或未启用 systray tag 时）
// execDirPath 参数被忽略
func StartSystray(ctx context.Context, cancel context.CancelFunc, execDirPath string) {
	// 空实现，不执行任何操作
}

// WaitForSystrayQuit 空实现（非 Windows 或未启用 systray tag 时）
func WaitForSystrayQuit() <-chan struct{} {
	// 返回一个永远不会发送信号的 channel
	ch := make(chan struct{})
	return ch
}

