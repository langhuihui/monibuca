package monica

import (
	"log"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

var (
	Summary = ServerSummary{}
)

type ServerSummary struct {
	Address string
	Memory  struct {
		Total uint64
		Free  uint64
		Used  uint64
		Usage float64
	}
	CPUUsage float64
	HardDisk struct {
		Total uint64
		Free  uint64
		Used  uint64
		Usage float64
	}
	NetWork     []NetWorkInfo
	Rooms       []*RoomInfo
	lastNetWork []NetWorkInfo
	ref         int
	control     chan bool
	reportChan  chan *ServerSummary
	Children    map[string]*ServerSummary
}
type NetWorkInfo struct {
	Name         string
	Receive      uint64
	Sent         uint64
	ReceiveSpeed uint64
	SentSpeed    uint64
}

func (s *ServerSummary) StartSummary() {
	ticker := time.NewTicker(time.Second)
	s.control = make(chan bool)
	s.reportChan = make(chan *ServerSummary)
	for {
		select {
		case <-ticker.C:
			if s.ref > 0 {
				Summary.collect()
			}
		case v := <-s.control:
			if v {
				if s.ref++; s.ref == 1 {
					log.Println("start report summary")
					OnSummaryHooks.Trigger(true)
				}
			} else {
				if s.ref--; s.ref == 0 {
					s.lastNetWork = nil
					log.Println("stop report summary")
					OnSummaryHooks.Trigger(false)
				}
			}
		case report := <-s.reportChan:
			s.Children[report.Address] = report
		}
	}
}
func (s *ServerSummary) Running() bool {
	return s.ref > 0
}
func (s *ServerSummary) Add() {
	s.control <- true
}
func (s *ServerSummary) Done() {
	s.control <- false
}
func (s *ServerSummary) Report(slave *ServerSummary) {
	s.reportChan <- slave
}
func (s *ServerSummary) collect() {
	v, _ := mem.VirtualMemory()
	//c, _ := cpu.Info()
	cc, _ := cpu.Percent(time.Second, false)
	d, _ := disk.Usage("/")
	//n, _ := host.Info()
	nv, _ := net.IOCounters(true)
	//boottime, _ := host.BootTime()
	//btime := time.Unix(int64(boottime), 0).Format("2006-01-02 15:04:05")
	s.Memory.Total = v.Total / 1024 / 1024
	s.Memory.Free = v.Available / 1024 / 1024
	s.Memory.Used = v.Used / 1024 / 1024
	s.Memory.Usage = v.UsedPercent
	//fmt.Printf("        Mem       : %v MB  Free: %v MB Used:%v Usage:%f%%\n", v.Total/1024/1024, v.Available/1024/1024, v.Used/1024/1024, v.UsedPercent)
	//if len(c) > 1 {
	//	for _, sub_cpu := range c {
	//		modelname := sub_cpu.ModelName
	//		cores := sub_cpu.Cores
	//		fmt.Printf("        CPU       : %v   %v cores \n", modelname, cores)
	//	}
	//} else {
	//	sub_cpu := c[0]
	//	modelname := sub_cpu.ModelName
	//	cores := sub_cpu.Cores
	//	fmt.Printf("        CPU       : %v   %v cores \n", modelname, cores)
	//}
	s.CPUUsage = cc[0]
	s.HardDisk.Free = d.Free / 1024 / 1024 / 1024
	s.HardDisk.Total = d.Total / 1024 / 1024 / 1024
	s.HardDisk.Used = d.Used / 1024 / 1024 / 1024
	s.HardDisk.Usage = d.UsedPercent
	s.NetWork = make([]NetWorkInfo, len(nv))
	for i, n := range nv {
		s.NetWork[i].Name = n.Name
		s.NetWork[i].Receive = n.BytesRecv
		s.NetWork[i].Sent = n.BytesSent
		if s.lastNetWork != nil && len(s.lastNetWork) > i {
			s.NetWork[i].ReceiveSpeed = n.BytesRecv - s.lastNetWork[i].Receive
			s.NetWork[i].SentSpeed = n.BytesSent - s.lastNetWork[i].Sent
		}
	}
	s.lastNetWork = s.NetWork
	//fmt.Printf("        Network: %v bytes / %v bytes\n", nv[0].BytesRecv, nv[0].BytesSent)
	//fmt.Printf("        SystemBoot:%v\n", btime)
	//fmt.Printf("        CPU Used    : used %f%% \n", cc[0])
	//fmt.Printf("        HD        : %v GB  Free: %v GB Usage:%f%%\n", d.Total/1024/1024/1024, d.Free/1024/1024/1024, d.UsedPercent)
	//fmt.Printf("        OS        : %v(%v)   %v  \n", n.Platform, n.PlatformFamily, n.PlatformVersion)
	//fmt.Printf("        Hostname  : %v  \n", n.Hostname)
	s.Rooms = nil
	AllRoom.Range(func(key interface{}, v interface{}) bool {
		s.Rooms = append(s.Rooms, &v.(*Room).RoomInfo)
		return true
	})
	return
}
