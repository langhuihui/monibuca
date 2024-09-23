package m7s

import "github.com/prometheus/client_golang/prometheus"

type prometheusDesc struct {
	CPUUsage *prometheus.Desc
	Memory   struct {
		Total, Used, Usage, Free *prometheus.Desc
	}
	Disk struct {
		Total, Used, Usage, Free *prometheus.Desc
	}
	Net struct {
		SendSpeed, ReceiveSpeed *prometheus.Desc
	}
	BPS, FPS, StreamCount, SubscribeCount, PullCount, PushCount, RecordCount, TransformCount *prometheus.Desc
}

func (d *prometheusDesc) init() {
	d.CPUUsage = prometheus.NewDesc("cpu_usage", "CPU usage", nil, nil)
	d.Memory.Total = prometheus.NewDesc("memory_total", "Memory total", nil, nil)
	d.Memory.Used = prometheus.NewDesc("memory_used", "Memory used", nil, nil)
	d.Memory.Usage = prometheus.NewDesc("memory_usage", "Memory usage", nil, nil)
	d.Memory.Free = prometheus.NewDesc("memory_free", "Memory free", nil, nil)
	d.Disk.Total = prometheus.NewDesc("disk_total", "Disk total", nil, nil)
	d.Disk.Used = prometheus.NewDesc("disk_used", "Disk used", nil, nil)
	d.Disk.Usage = prometheus.NewDesc("disk_usage", "Disk usage", nil, nil)
	d.Disk.Free = prometheus.NewDesc("disk_free", "Disk free", nil, nil)
	d.Net.SendSpeed = prometheus.NewDesc("net_send_speed", "Net send speed", []string{"name"}, nil)
	d.Net.ReceiveSpeed = prometheus.NewDesc("net_receive_speed", "Net receive speed", []string{"name"}, nil)
	d.BPS = prometheus.NewDesc("bps", "Bytes Per Second", []string{"streamPath", "pluginName", "trackType"}, nil)
	d.FPS = prometheus.NewDesc("fps", "Frames Per Second", []string{"streamPath", "pluginName", "trackType"}, nil)
	d.StreamCount = prometheus.NewDesc("stream_count", "Stream count", nil, nil)
	d.SubscribeCount = prometheus.NewDesc("subscribe_count", "Subscribe count", nil, nil)
	d.PullCount = prometheus.NewDesc("pull_count", "Pull count", nil, nil)
	d.PushCount = prometheus.NewDesc("push_count", "Push count", nil, nil)
	d.RecordCount = prometheus.NewDesc("record_count", "Record count", nil, nil)
	d.TransformCount = prometheus.NewDesc("transform_count", "Transform count", nil, nil)
}

func (s *Server) Describe(ch chan<- *prometheus.Desc) {
	desc := s.prometheusDesc
	ch <- desc.BPS
	ch <- desc.FPS
	ch <- desc.CPUUsage
	ch <- desc.Memory.Total
	ch <- desc.Memory.Used
	ch <- desc.Memory.Usage
	ch <- desc.Memory.Free
	ch <- desc.Disk.Total
	ch <- desc.Disk.Used
	ch <- desc.Disk.Usage
	ch <- desc.Disk.Free
	ch <- desc.Net.SendSpeed
	ch <- desc.StreamCount
	ch <- desc.SubscribeCount
	ch <- desc.PullCount
	ch <- desc.PushCount
	ch <- desc.RecordCount
	ch <- desc.TransformCount
}

func (s *Server) Collect(ch chan<- prometheus.Metric) {
	if s.lastSummary != nil {
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.CPUUsage, prometheus.GaugeValue, float64(s.lastSummary.CpuUsage))
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Memory.Total, prometheus.GaugeValue, float64(s.lastSummary.Memory.Total))
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Memory.Used, prometheus.GaugeValue, float64(s.lastSummary.Memory.Used))
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Memory.Usage, prometheus.GaugeValue, float64(s.lastSummary.Memory.Usage))
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Memory.Free, prometheus.GaugeValue, float64(s.lastSummary.Memory.Free))
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Disk.Total, prometheus.GaugeValue, float64(s.lastSummary.HardDisk.Total))
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Disk.Used, prometheus.GaugeValue, float64(s.lastSummary.HardDisk.Used))
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Disk.Usage, prometheus.GaugeValue, float64(s.lastSummary.HardDisk.Usage))
		ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Disk.Free, prometheus.GaugeValue, float64(s.lastSummary.HardDisk.Free))
		for _, net := range s.lastSummary.NetWork {
			ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Net.SendSpeed, prometheus.GaugeValue, float64(net.SentSpeed), net.Name)
			ch <- prometheus.MustNewConstMetric(s.prometheusDesc.Net.ReceiveSpeed, prometheus.GaugeValue, float64(net.ReceiveSpeed), net.Name)
		}
	}
	s.Call(func() error {
		for stream := range s.Streams.Range {
			ch <- prometheus.MustNewConstMetric(s.prometheusDesc.BPS, prometheus.GaugeValue, float64(stream.VideoTrack.AVTrack.BPS), stream.StreamPath, stream.Plugin.Meta.Name, "video")
			ch <- prometheus.MustNewConstMetric(s.prometheusDesc.FPS, prometheus.GaugeValue, float64(stream.VideoTrack.AVTrack.FPS), stream.StreamPath, stream.Plugin.Meta.Name, "video")
			ch <- prometheus.MustNewConstMetric(s.prometheusDesc.BPS, prometheus.GaugeValue, float64(stream.AudioTrack.AVTrack.BPS), stream.StreamPath, stream.Plugin.Meta.Name, "audio")
			ch <- prometheus.MustNewConstMetric(s.prometheusDesc.FPS, prometheus.GaugeValue, float64(stream.AudioTrack.AVTrack.FPS), stream.StreamPath, stream.Plugin.Meta.Name, "audio")
		}
		return nil
	})
}
