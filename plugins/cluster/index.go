package cluster

import (
	"bufio"
	"encoding/json"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	. "github.com/langhuihui/monibuca/monica"
)

const (
	_ byte = iota
	MSG_AUDIO
	MSG_VIDEO
	MSG_SUBSCRIBE
	MSG_AUTH
	MSG_SUMMARY
	MSG_LOG
)

var (
	config = struct {
		Master     string
		ListenAddr string
	}{}
	slaves     = sync.Map{}
	masterConn *net.TCPConn
)

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "Cluster",
		Type:   PLUGIN_HOOK | PLUGIN_PUBLISHER | PLUGIN_SUBSCRIBER,
		Config: &config,
		Run:    run,
	})
}
func run() {
	if config.Master != "" {
		OnSubscribeHooks.AddHook(onSubscribe)
		addr, err := net.ResolveTCPAddr("tcp", config.Master)
		if MayBeError(err) {
			return
		}
		masterConn, err = net.DialTCP("tcp", nil, addr)
		if MayBeError(err) {
			return
		}
		go readMaster()
	}
	if config.ListenAddr != "" {
		OnSummaryHooks.AddHook(onSummary)
		log.Printf("server bare start at %s", config.ListenAddr)
		log.Fatal(ListenBare(config.ListenAddr))
	}
}
func readMaster() {
	var err error
	defer func() {
		for {
			time.Sleep(time.Second*5 + time.Duration(rand.Int63n(5))*time.Second)
			addr, _ := net.ResolveTCPAddr("tcp", config.Master)
			if masterConn, err = net.DialTCP("tcp", nil, addr); err == nil {
				go readMaster()
				return
			}
		}
	}()
	brw := bufio.NewReadWriter(bufio.NewReader(masterConn), bufio.NewWriter(masterConn))
	//首次报告
	if b, err := json.Marshal(Summary); err == nil {
		_, err = masterConn.Write(b)
	}
	for {
		cmd, err := brw.ReadByte()
		if err != nil {
			return
		}
		switch cmd {
		case MSG_SUMMARY: //收到主服务器指令，进行采集和上报
			if cmd, err = brw.ReadByte(); err != nil {
				return
			}
			if cmd == 1 {
				Summary.Add()
				go onReport()
			} else {
				Summary.Done()
			}
		}
	}
}

//定时上报
func onReport() {
	for range time.NewTicker(time.Second).C {
		if Summary.Running() {
			if b, err := json.Marshal(Summary); err == nil {
				data := make([]byte, len(b)+2)
				data[0] = MSG_SUMMARY
				copy(data[1:], b)
				data[len(data)-1] = 0
				_, err = masterConn.Write(data)
			}
		} else {
			return
		}
	}
}

//通知从服务器需要上报或者关闭上报
func onSummary(start bool) {
	slaves.Range(func(k, v interface{}) bool {
		conn := v.(*net.TCPConn)
		b := []byte{MSG_SUMMARY, 0}
		if start {
			b[1] = 1
		}
		conn.Write(b)
		return true
	})
}

func onSubscribe(s *OutputStream) {
	if s.Publisher == nil {
		go PullUpStream(s.StreamPath)
	}
}
