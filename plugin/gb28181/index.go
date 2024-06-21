package plugin_gb28181

import (
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"m7s.live/m7s/v5"
)

type SipConfig struct {
	ListenAddr    []string
	ListenTLSAddr []string
}

type GB28181Plugin struct {
	m7s.Plugin
	Sip    SipConfig
	ua     *sipgo.UserAgent
	server *sipgo.Server
}

var _ = m7s.InstallPlugin[GB28181Plugin]()

func (gb *GB28181Plugin) OnInit() (err error) {
	gb.ua, err = sipgo.NewUA()              // Build user agent
	gb.server, err = sipgo.NewServer(gb.ua) // Creating server handle for ua
	gb.server.OnRegister(gb.OnRegister)
	gb.server.ListenAndServe(gb, "tcp", "")
	return
}

func (gb *GB28181Plugin) OnRegister(req *sip.Request, tx sip.ServerTransaction) {

}
