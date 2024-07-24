package plugin_gb28181

import (
	"fmt"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
	"github.com/rs/zerolog/log"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	gb28181 "m7s.live/m7s/v5/plugin/gb28181/pkg"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type SipConfig struct {
	ListenAddr    []string
	ListenTLSAddr []string
}

type GB28181Plugin struct {
	m7s.Plugin
	Username string
	Password string
	Sip      SipConfig
	ua       *sipgo.UserAgent
	server   *sipgo.Server
	devices  util.Collection[string, *gb28181.Device]
}

var _ = m7s.InstallPlugin[GB28181Plugin]()

func (gb *GB28181Plugin) OnInit() (err error) {
	gb.ua, err = sipgo.NewUA(sipgo.WithUserAgent("monibuca" + m7s.Version)) // Build user agent
	gb.server, err = sipgo.NewServer(gb.ua)                                 // Creating server handle for ua
	gb.server.OnRegister(gb.OnRegister)
	gb.server.OnMessage(gb.OnMessage)
	gb.devices.L = new(sync.RWMutex)
	go gb.server.ListenAndServe(gb, "tcp", "")
	return
}

func (gb *GB28181Plugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/ps/replay/{streamPath...}": gb.api_ps_replay,
	}
}

func (gb *GB28181Plugin) OnRegister(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from.Address.User == "" {
		gb.Error("OnRegister", "error", "no user")
		return
	}
	isUnregister := false
	id := from.Address.User
	exp := req.GetHeader("Expires")
	if exp == nil {
		gb.Error("OnRegister", "error", "no expires")
		return
	}
	expSec, err := strconv.ParseInt(exp.Value(), 10, 32)
	if err != nil {
		gb.Error("OnRegister", "error", err.Error())
		return
	}
	if expSec == 0 {
		isUnregister = true
	}
	// 不需要密码情况
	if gb.Username != "" && gb.Password != "" {
		h := req.GetHeader("Authorization")
		var chal digest.Challenge
		var cred *digest.Credentials
		var digCred *digest.Credentials
		if h == nil {
			chal = digest.Challenge{
				Realm:     "monibuca-server",
				Nonce:     fmt.Sprintf("%d", time.Now().UnixMicro()),
				Opaque:    "monibuca",
				Algorithm: "MD5",
			}

			res := sip.NewResponseFromRequest(req, http.StatusUnauthorized, "Unathorized", nil)
			res.AppendHeader(sip.NewHeader("WWW-Authenticate", chal.String()))

			err = tx.Respond(res)
			return
		}

		cred, err = digest.ParseCredentials(h.Value())
		if err != nil {
			log.Error().Err(err).Msg("parsing creds failed")
			err = tx.Respond(sip.NewResponseFromRequest(req, http.StatusUnauthorized, "Bad credentials", nil))
			return
		}

		// Check registry
		if cred.Username != gb.Username {
			err = tx.Respond(sip.NewResponseFromRequest(req, http.StatusNotFound, "Bad authorization header", nil))
			return
		}

		// Make digest and compare response
		digCred, err = digest.Digest(&chal, digest.Options{
			Method:   "REGISTER",
			URI:      cred.URI,
			Username: gb.Username,
			Password: gb.Password,
		})

		if err != nil {
			gb.Error("Calc digest failed")
			err = tx.Respond(sip.NewResponseFromRequest(req, http.StatusUnauthorized, "Bad credentials", nil))
			return
		}

		if cred.Response != digCred.Response {
			err = tx.Respond(sip.NewResponseFromRequest(req, http.StatusUnauthorized, "Unathorized", nil))
			return
		}
		err = tx.Respond(sip.NewResponseFromRequest(req, http.StatusOK, "OK", nil))
	}
	var d *gb28181.Device
	if isUnregister {
		if gb.devices.RemoveByKey(id) {
			gb.Info("Unregister Device", "id", id)
		} else {
			return
		}
	} else {
		var ok bool
		if d, ok = gb.devices.Get(id); ok {
			gb.RecoverDevice(d, req)
		} else {
			d = gb.StoreDevice(id, req)
		}
	}
	DeviceNonce.Delete(id)
	DeviceRegisterCount.Delete(id)
	if !isUnregister {
		//订阅设备更新
		go d.syncChannels()
	}
}

func (gb *GB28181Plugin) OnMessage(req *sip.Request, tx sip.ServerTransaction) {

}
