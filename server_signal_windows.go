//go:build windows

package m7s

import "os"

// setupDumpSignal returns a channel that never receives on Windows
// (SIGUSR1 is not available). On-demand goroutine dumps must be triggered
// via the CPU watchdog automatic threshold instead.
func setupDumpSignal() (<-chan os.Signal, func()) {
	return make(chan os.Signal), func() {}
}
