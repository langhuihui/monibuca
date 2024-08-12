package plugin_rtsp

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	. "m7s.live/m7s/v5/plugin/rtsp/pkg"
)

const defaultConfig = m7s.DefaultYaml(`tcp:
  listenaddr: :554`)

var _ = m7s.InstallPlugin[RTSPPlugin](defaultConfig, Pull, Push)

type RTSPPlugin struct {
	m7s.Plugin
}

func (p *RTSPPlugin) GetPullableList() []string {
	return slices.Collect(maps.Keys(p.GetCommonConf().PullOnSub))
}

func (p *RTSPPlugin) OnInit() error {
	for streamPath, url := range p.GetCommonConf().PullOnStart {
		p.Pull(streamPath, url)
	}
	return nil
}

func (p *RTSPPlugin) OnTCPConnect(conn *net.TCPConn) {
	logger := p.Logger.With("remote", conn.RemoteAddr().String())
	var receiver *Receiver
	var sender *Sender
	var err error
	nc := NewNetConnection(conn)
	nc.Logger = logger
	defer func() {
		nc.Destroy()
		if p := recover(); p != nil {
			err = p.(error)
			logger.Error(err.Error(), "stack", string(debug.Stack()))
		}
		if receiver != nil {
			receiver.Stop(err)
		}
	}()
	var req *util.Request
	var sendMode bool
	for {
		req, err = nc.ReadRequest()
		if err != nil {
			return
		}

		if nc.URL == nil {
			nc.URL = req.URL
			logger = logger.With("url", nc.URL.String())
			nc.UserAgent = req.Header.Get("User-Agent")
			logger.Info("connect", "userAgent", nc.UserAgent)
		}

		//if !c.auth.Validate(req) {
		//	res := &tcp.Response{
		//		Status:  "401 Unauthorized",
		//		Header:  map[string][]string{"Www-Authenticate": {`Basic realm="go2rtc"`}},
		//		Request: req,
		//	}
		//	if err = c.WriteResponse(res); err != nil {
		//		return err
		//	}
		//	continue
		//}

		// Receiver: OPTIONS > DESCRIBE > SETUP... > PLAY > TEARDOWN
		// Sender: OPTIONS > ANNOUNCE > SETUP... > RECORD > TEARDOWN
		switch req.Method {
		case MethodOptions:
			res := &util.Response{
				Header: map[string][]string{
					"Public": {"OPTIONS, SETUP, TEARDOWN, DESCRIBE, PLAY, PAUSE, ANNOUNCE, RECORD"},
				},
				Request: req,
			}
			if err = nc.WriteResponse(res); err != nil {
				return
			}

		case MethodAnnounce:
			if req.Header.Get("Content-Type") != "application/sdp" {
				err = errors.New("wrong content type")
				return
			}

			nc.SDP = string(req.Body) // for info
			var medias []*Media
			if medias, err = UnmarshalSDP(req.Body); err != nil {
				return
			}

			receiver = &Receiver{}
			receiver.NetConnection = nc
			if receiver.Publisher, err = p.Publish(nc, strings.TrimPrefix(nc.URL.Path, "/")); err != nil {
				receiver = nil
				err = nc.WriteResponse(&util.Response{
					StatusCode: 500, Status: err.Error(),
				})
				return
			}
			if err = receiver.SetMedia(medias); err != nil {
				return
			}
			res := &util.Response{Request: req}
			if err = nc.WriteResponse(res); err != nil {
				return
			}

		case MethodDescribe:
			sendMode = true
			sender = &Sender{}
			sender.NetConnection = nc
			sender.Subscriber, err = p.Subscribe(nc, strings.TrimPrefix(nc.URL.Path, "/"))
			if err != nil {
				res := &util.Response{
					StatusCode: http.StatusBadRequest,
					Status:     err.Error(),
					Request:    req,
				}
				_ = nc.WriteResponse(res)
				return
			}
			res := &util.Response{
				Header: map[string][]string{
					"Content-Type": {"application/sdp"},
				},
				Request: req,
			}
			// convert tracks to real output medias
			var medias []*Media
			if medias, err = sender.GetMedia(); err != nil {
				return
			}
			if res.Body, err = MarshalSDP(nc.SessionName, medias); err != nil {
				return
			}

			nc.SDP = string(res.Body) // for info

			if err = nc.WriteResponse(res); err != nil {
				return
			}

		case MethodSetup:
			tr := req.Header.Get("Transport")

			res := &util.Response{
				Header:  map[string][]string{},
				Request: req,
			}

			const transport = "RTP/AVP/TCP;unicast;interleaved="
			if strings.HasPrefix(tr, transport) {

				nc.Session = util.RandomString(10)

				if sendMode {
					if i := reqTrackID(req); i >= 0 {
						tr = fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", i*2, i*2+1)
						res.Header.Set("Transport", tr)
					} else {
						res.Status = "400 Bad Request"
					}
				} else {
					res.Header.Set("Transport", tr[:len(transport)+3])
				}
			} else {
				res.Status = "461 Unsupported transport"
			}

			if err = nc.WriteResponse(res); err != nil {
				return
			}

		case MethodRecord:
			res := &util.Response{Request: req}
			if err = nc.WriteResponse(res); err != nil {
				return
			}
			err = receiver.Receive()
			return
		case MethodPlay:
			res := &util.Response{Request: req}
			if err = nc.WriteResponse(res); err != nil {
				return
			}
			err = sender.Send()
			return
		case MethodTeardown:
			res := &util.Response{Request: req}
			_ = nc.WriteResponse(res)
			return

		default:
			p.Warn("unsupported method", "method", req.Method)
		}
	}
}

func reqTrackID(req *util.Request) int {
	var s string
	if req.URL.RawQuery != "" {
		s = req.URL.RawQuery
	} else {
		s = req.URL.Path
	}
	if i := strings.LastIndexByte(s, '='); i > 0 {
		if i, err := strconv.Atoi(s[i+1:]); err == nil {
			return i
		}
	}
	return -1
}
