package plugin_rtsp

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"m7s.live/v5/pkg/util"
	. "m7s.live/v5/plugin/rtsp/pkg"
)

type RTSPServer struct {
	*NetConnection
	conf *RTSPPlugin
}

func (task *RTSPServer) Go() (err error) {
	var receiver *Receiver
	var sender *Sender
	var req *util.Request
	var sendMode bool
	for {
		req, err = task.ReadRequest()
		if err != nil {
			return
		}

		if task.URL == nil {
			task.URL = req.URL
			task.Logger = task.With("url", task.URL.String())
			task.UserAgent = req.Header.Get("User-Agent")
			task.Info("connect", "userAgent", task.UserAgent)
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
			if err = task.WriteResponse(res); err != nil {
				return
			}

		case MethodAnnounce:
			if req.Header.Get("Content-Type") != "application/sdp" {
				err = errors.New("wrong content type")
				return
			}

			task.SDP = string(req.Body) // for info
			var medias []*Media
			if medias, err = UnmarshalSDP(req.Body); err != nil {
				return
			}

			receiver = &Receiver{}
			receiver.NetConnection = task.NetConnection
			if receiver.Publisher, err = task.conf.Publish(task, strings.TrimPrefix(task.URL.Path, "/")); err != nil {
				receiver = nil
				err = task.WriteResponse(&util.Response{
					StatusCode: 500, Status: err.Error(),
				})
				return
			}
			receiver.Publisher.RemoteAddr = task.Conn.RemoteAddr().String()
			if err = receiver.SetMedia(medias); err != nil {
				return
			}
			res := &util.Response{Request: req}
			if err = task.WriteResponse(res); err != nil {
				return
			}
			task.Depend(receiver.Publisher)
		case MethodDescribe:
			sendMode = true
			sender = &Sender{}
			sender.NetConnection = task.NetConnection
			rawQuery := req.URL.RawQuery
			streamPath := strings.TrimPrefix(task.URL.Path, "/")
			if rawQuery != "" {
				streamPath += "?" + rawQuery
			}
			sender.Subscriber, err = task.conf.Subscribe(task, streamPath)
			if err != nil {
				res := &util.Response{
					StatusCode: http.StatusBadRequest,
					Status:     err.Error(),
					Request:    req,
				}
				_ = task.WriteResponse(res)
				return
			}
			sender.Subscriber.RemoteAddr = task.Conn.RemoteAddr().String()
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
			if res.Body, err = MarshalSDP(task.SessionName, medias); err != nil {
				return
			}

			task.SDP = string(res.Body) // for info

			if err = task.WriteResponse(res); err != nil {
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

				task.Session = util.RandomString(10)

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

			if err = task.WriteResponse(res); err != nil {
				return
			}

		case MethodRecord:
			res := &util.Response{Request: req}
			if err = task.WriteResponse(res); err != nil {
				return
			}
			err = receiver.Receive()
			return
		case MethodPlay:
			res := &util.Response{Request: req}
			if err = task.WriteResponse(res); err != nil {
				return
			}
			err = sender.Send()
			return
		case MethodTeardown:
			res := &util.Response{Request: req}
			_ = task.WriteResponse(res)
			return

		default:
			task.Warn("unsupported method", "method", req.Method)
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
