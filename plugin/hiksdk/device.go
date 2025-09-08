package plugin_hiksdk

import (
	"fmt"
	"strings"

	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/hiksdk/pkg"

	"github.com/prometheus/client_golang/prometheus"
)

type HikDevice struct {
	task.Job
	IP       string
	UserName string
	Password string
	Port     int
	Device   pkg.Device
	Conf     *HikPlugin
}

func (d *HikDevice) Start() (err error) {
	info := pkg.DeviceInfo{
		IP:       d.IP,
		UserName: d.UserName,
		Password: d.Password,
		Port:     d.Port,
	}
	d.Device = pkg.NewHKDevice(info)
	return
}

func (d *HikDevice) Run() (err error) {
	if _, err := d.Device.Login(); err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("success login")
	}
	deviceInfo, err := d.Device.GetDeiceInfo() // 获取设备参数
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println(deviceInfo)
	}

	channelNames, err := d.Device.GetChannelName() // 获取通道
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("通道:", channelNames)
	}
	d.AutoPullStream()
	return
}


func (d *HikDevice) AutoPullStream() {
	deviceInfo, _ := d.Device.GetDeiceInfo()     // 获取设备参数
	channelNames, _ := d.Device.GetChannelName() // 获取通道
	for i := 1; i <= int(deviceInfo.ByChanNum); i++ {
		d.PullStream(deviceInfo.DeviceID, channelNames[i], i)
	}
}

func (d *HikDevice) PullStream(ifname string, channelName string, channelId int) error {
	// 生成流路径
	ifname = strings.ReplaceAll(ifname, "-", "_")
	streamPath := fmt.Sprintf("%s/%s", ifname, channelName)
	receiver := &pkg.PSReceiver{}
	receiver.Device = d.Device
	receiver.ChannelId = channelId
	receiver.Publisher, _ = d.Conf.Publish(d.Conf, streamPath)
	go d.Conf.RunTask(receiver)
	return nil
}

func (d *HikDevice) Describe(ch chan<- *prometheus.Desc) {
	d.Device.Logout()
}
