//go:build !windows

package m7s

import (
	"os"
	"os/signal"
	"syscall"
)

// setupDumpSignal returns a channel that receives SIGUSR1 on Unix systems
// so operators can trigger an on-demand goroutine dump with: kill -USR1 <pid>
func setupDumpSignal() (<-chan os.Signal, func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)
	return sigCh, func() { signal.Stop(sigCh) }
}
