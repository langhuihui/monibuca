package plugin_hiksdk

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	m7s "m7s.live/v5"
	"m7s.live/v5/plugin/hiksdk/pkg"
)

type HikPlugin struct {
	m7s.Plugin
	Clients []Client `yaml:"client,omitempty"` //共享的通道，格式为GBID，流地址
	list    []HikDevice
}

type Client struct {
	IP       string `yaml:"ip"`
	UserName string `yaml:"username"`
	Password string `yaml:"password"`
	Port     int    `yaml:"port"`
}

var _ = m7s.InstallPlugin[HikPlugin](m7s.PluginMeta{
	NewPuller: NewPuller,
	NewPusher: NewPusher,
})

func init() {
	fmt.Println("success 初始化海康SDK")
	pkg.InitHikSDK()
}

func (hik *HikPlugin) Start() (err error) {
	for i, client := range hik.Clients {
		fmt.Printf("Client[%d]: IP=%s, UserName=%s, Password=%s, Port=%d\n", i, client.IP, client.UserName, client.Password, client.Port)
		device := HikDevice{
			IP:       client.IP,
			UserName: client.UserName,
			Password: client.Password,
			Port:     client.Port,
			Conf:     hik,
		}
		hik.list = append(hik.list, device)
	}
	for _, device := range hik.list {
		go hik.AddTask(&device)
	}
	return
}

func (hik *HikPlugin) Describe(ch chan<- *prometheus.Desc) {
	pkg.HKExit()
}
