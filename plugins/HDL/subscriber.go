package HDL

import (
	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/avformat"
	"github.com/langhuihui/monibuca/monica/pool"
	"log"
	"net/http"
	"strings"
)

var config = new(ListenerConfig)

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "HDL",
		Type:   PLUGIN_SUBSCRIBER,
		Config: config,
		Run:    run,
	})
}

func run() {
	log.Printf("HDL start at %s", config.ListenAddr)
	log.Fatal(http.ListenAndServe(config.ListenAddr, http.HandlerFunc(HDLHandler)))
}

func HDLHandler(w http.ResponseWriter, r *http.Request) {
	sign := r.URL.Query().Get("sign")
	if err := AuthHooks.Trigger(sign); err != nil {
		w.WriteHeader(403)
		return
	}
	stringPath := strings.TrimLeft(r.RequestURI, "/")
	if strings.HasSuffix(stringPath, ".flv") {
		stringPath = strings.TrimRight(stringPath, ".flv")
	}
	if _, ok := AllRoom.Load(stringPath); ok {
		//atomic.AddInt32(&hdlId, 1)
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Content-Type", "video/x-flv")
		w.Write(avformat.FLVHeader)
		p := OutputStream{
			Sign: sign,
			SendHandler: func(packet *pool.SendPacket) error {
				return avformat.WriteFLVTag(w, packet)
			},
			SubscriberInfo: SubscriberInfo{
				ID: r.RemoteAddr, Type: "FLV",
			},
		}
		p.Play(stringPath)
	} else {
		w.WriteHeader(404)
	}
}
