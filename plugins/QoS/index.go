package QoS

import (
	. "github.com/langhuihui/monibuca/monica"
)

var (
	selectMap = map[string][]string{
		"low":    {"low", "medium", "high"},
		"medium": {"medium", "low", "high"},
		"high":   {"high", "medium", "low"},
	}
)

func getQualityName(name string, qualityLevel string) string {
	if qualityLevel == "" {
		return name
	}
	for _, l := range selectMap[qualityLevel] {
		if _, ok := AllRoom.Load(name + "/" + l); ok {
			return name + "/" + l
		}
	}
	return name + "/" + qualityLevel
}

var config = struct {
	Suffix []string
}{}

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "QoS",
		Type:   PLUGIN_HOOK,
		Config: &config,
		Run:    run,
	})
}
func run() {
	OnDropHooks.AddHook(func(s *OutputStream) {
		if s.TotalDrop > s.TotalPacket>>2 {
			//TODO
			//s.Control<-&ChangeRoomCmd{s,AllRoom.Get()}
		}
	})
}
