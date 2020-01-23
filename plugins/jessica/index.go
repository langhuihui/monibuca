package jessica

import (
	. "github.com/langhuihui/monibuca/monica"
	"log"
	"net/http"
)

var config = new(ListenerConfig)

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "Jessica",
		Type:   PLUGIN_SUBSCRIBER,
		Config: config,
		Run:    run,
	})
}
func run() {
	log.Printf("server Jessica start at %s", config.ListenAddr)
	log.Fatal(http.ListenAndServe(config.ListenAddr, http.HandlerFunc(WsHandler)))
}
