package QoS

import (
	"strings"

	. "github.com/langhuihui/monibuca/monica"
)

// var (
// 	selectMap = map[string][]string{
// 		"low":    {"low", "medium", "high"},
// 		"medium": {"medium", "low", "high"},
// 		"high":   {"high", "medium", "low"},
// 	}
// )

// func getQualityName(name string, qualityLevel string) string {
// 	for _, l := range selectMap[qualityLevel] {
// 		if _, ok := AllRoom.Load(name + "/" + l); ok {
// 			return name + "/" + l
// 		}
// 	}
// 	return name + "/" + qualityLevel
// }

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
			var newStreamPath = ""
			for i, suf := range config.Suffix {
				if strings.HasSuffix(s.StreamPath, suf) {
					if i < len(config.Suffix)-1 {
						newStreamPath = s.StreamPath + "/" + config.Suffix[i+1]
						break
					}
				} else {
					newStreamPath = s.StreamPath + "/" + suf
					break
				}
			}
			if newStreamPath != "" {
				if _, ok := AllRoom.Load(newStreamPath); ok {
					s.Control <- &ChangeRoomCmd{s, AllRoom.Get(newStreamPath)}
				}
			}
		}
	})
}
