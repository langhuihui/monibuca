package plugin_gb28181pro

import (
	"context"
	"strings"
	"time"
)

const defaultSIPUDPRecycleInterval = 12 * time.Hour

func isUDPNetwork(network string) bool {
	switch strings.ToLower(network) {
	case "udp", "udp4", "udp6":
		return true
	default:
		return false
	}
}

// startSIPListener 启动 SIP 监听。
// UDP 走定期回收：sipgo 在 UDP 读循环里会累积远端地址字符串且监听不退出时不释放，
// 在 monibuca 侧重绑 UDP 监听可让该 goroutine 退出并释放内存，无需改 sipgo 源码。
func (gb *GB28181Plugin) startSIPListener(network, addr string) {
	network = strings.ToLower(network)
	if isUDPNetwork(network) {
		go gb.runSIPUDPListenerWithRecycle(network, addr)
		return
	}
	go func() {
		if err := gb.server.ListenAndServe(gb, network, addr); err != nil {
			gb.Error("SIP listener stopped", "network", network, "addr", addr, "err", err)
		}
	}()
}

func (gb *GB28181Plugin) sipUDPRecycleInterval() time.Duration {
	if gb.Sip.DisableUDPRecycle {
		return 0
	}
	if gb.Sip.UDPRecycleInterval > 0 {
		return gb.Sip.UDPRecycleInterval
	}
	return defaultSIPUDPRecycleInterval
}

func (gb *GB28181Plugin) runSIPUDPListenerWithRecycle(network, addr string) {
	interval := gb.sipUDPRecycleInterval()
	for {
		select {
		case <-gb.Done():
			return
		default:
		}

		listenCtx, cancel := context.WithCancel(gb)
		done := make(chan struct{})
		go func() {
			defer close(done)
			if err := gb.server.ListenAndServe(listenCtx, network, addr); err != nil {
				gb.Warn("SIP UDP listener stopped", "network", network, "addr", addr, "err", err)
			}
		}()

		if interval <= 0 {
			select {
			case <-gb.Done():
				cancel()
				<-done
			case <-done:
			}
			return
		}

		timer := time.NewTimer(interval)
		select {
		case <-gb.Done():
			timer.Stop()
			cancel()
			<-done
			return
		case <-timer.C:
			gb.Info("recycling SIP UDP listener", "addr", addr, "interval", interval)
			cancel()
			<-done
			time.Sleep(time.Second)
		}
	}
}
