package cluster

import (
	. "github.com/langhuihui/monibuca/monica"
	"log"
)

const (
	_ byte = iota
	MSG_AUDIO
	MSG_VIDEO
	MSG_SUBSCRIBE
	MSG_AUTH
)

var config = struct {
	Master     string
	ListenAddr string
}{}

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
	}
	if config.ListenAddr != "" {
		log.Printf("server bare start at %s", config.ListenAddr)
		log.Fatal(ListenBare(config.ListenAddr))
	}
}

func onSubscribe(s *OutputStream) {
	if s.Publisher == nil {
		go PullUpStream(s.StreamName)
	}
}
