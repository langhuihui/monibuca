package plugin_rtsp

import (
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"regexp"
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

	// 添加延迟函数在方法结束时清理资源
	defer func() {
		if receiver != nil {
			receiver.Dispose()
		}
		if sender != nil {
			sender.Dispose()
		}
		// 确保任何残留资源被清理
		if task.NetConnection != nil {
			task.NetConnection.Dispose()
		}
		task.Info("RTSP connection closed and resources cleaned up")
	}()

	for {
		req, err = task.ReadRequest()
		if err != nil {
			return
		}

		if task.URL == nil {
			task.URL = req.URL
			task.Logger = task.Logger.With("url", task.URL.String())
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
				Header: textproto.MIMEHeader{
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
			receiver.Publisher.Using(task)
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
				Header: textproto.MIMEHeader{
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
				Header:  textproto.MIMEHeader{},
				Request: req,
			}

			// TCP传输模式
			const tcpTransport = "RTP/AVP/TCP;unicast;interleaved="
			// UDP传输模式前缀
			const udpTransport = "RTP/AVP"

			if strings.HasPrefix(tr, tcpTransport) {
				task.Debug("into tcp play")
				// 原有的TCP传输处理逻辑
				task.Session = util.RandomString(10)

				if sendMode {
					if i := reqTrackID(req); i >= 0 {
						tr = fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", i*2, i*2+1)
						res.Header.Set("Transport", tr)
					} else {
						res.Status = "400 Bad Request"
					}
				} else {
					res.Header.Set("Transport", tr[:len(tcpTransport)+3])
				}
			} else if strings.HasPrefix(tr, udpTransport) && strings.Contains(tr, "unicast") && strings.Contains(tr, "client_port=") {
				task.Debug("into udp play")

				// UDP传输处理逻辑
				task.Session = util.RandomString(10)

				if sendMode {
					if i := reqTrackID(req); i >= 0 {
						// 解析客户端请求的端口
						clientPortsRe := regexp.MustCompile(`client_port=(\d+)-(\d+)`)
						matches := clientPortsRe.FindStringSubmatch(tr)

						if len(matches) == 3 {
							clientRTPPort, _ := strconv.Atoi(matches[1])
							clientRTCPPort, _ := strconv.Atoi(matches[2])

							// 从端口池获取服务器端口
							serverRTPPort, err := task.conf.GetUDPPort()
							if err != nil {
								task.Error("Failed to get UDP port from pool", "error", err)
								res.Status = "500 Internal Server Error: No available UDP ports"
								break
							}
							serverRTCPPort, err := task.conf.GetUDPPort()
							if err != nil {
								task.Error("Failed to get UDP port from pool", "error", err)
								res.Status = "500 Internal Server Error: No available UDP ports"
								break
							}
							// 在sender中记录这些端口信息
							if sender.UDPPorts == nil {
								sender.UDPPorts = make(map[int][]int)
							}
							// 保存 [clientRTP, clientRTCP, serverRTP, serverRTCP]
							sender.UDPPorts[i] = []int{clientRTPPort, clientRTCPPort, int(serverRTPPort), int(serverRTCPPort)}

							// 记录分配的端口，用于在连接结束时释放
							if sender.AllocatedUDPPorts == nil {
								sender.AllocatedUDPPorts = make([]uint16, 0)
							}
							sender.AllocatedUDPPorts = append(sender.AllocatedUDPPorts, serverRTPPort)

							// 设置插件引用，用于在Dispose时释放端口

							// 设置传输响应
							udpResponse := fmt.Sprintf("RTP/AVP;unicast;client_port=%d-%d;server_port=%d-%d",
								clientRTPPort, clientRTCPPort, serverRTPPort, serverRTCPPort)
							res.Header.Set("Transport", udpResponse)

							// 标记为UDP传输模式
							task.Transport = "UDP"
						} else {
							res.Status = "400 Bad Request: Invalid client_port format"
						}
					} else {
						res.Status = "400 Bad Request: Invalid track ID"
					}
				} else {
					res.Status = "400 Bad Request: UDP only supported for PLAY mode"
				}
			} else {
				res.Status = "461 Unsupported transport"
			}

			// 设置Session头
			res.Header.Set("Session", task.Session)

			if err = task.WriteResponse(res); err != nil {
				return
			}

		case MethodRecord:
			res := &util.Response{Request: req}
			if err = task.WriteResponse(res); err != nil {
				task.Error("Failed to write response", "error", err)
				return
			}
			task.Info("Starting RTSP record session")
			err = receiver.Receive()
			if err != nil {
				task.Error("RTSP receive error", "error", err)
			}
			return
		case MethodPlay:
			res := &util.Response{Request: req}
			if err = task.WriteResponse(res); err != nil {
				task.Error("Failed to write response", "error", err)
				return
			}
			task.Info("Starting RTSP play session")
			err = sender.Send()
			if err != nil {
				task.Error("RTSP send error", "error", err)
			}
			return
		case MethodTeardown:
			res := &util.Response{Request: req}
			_ = task.WriteResponse(res)
			task.Info("RTSP teardown received")
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
