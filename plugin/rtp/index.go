package plugin_rtp

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pion/rtp"
	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	mpegps "m7s.live/v5/pkg/format/ps"
	"m7s.live/v5/pkg/util"
	pb "m7s.live/v5/plugin/rtp/pb"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

type RTPPlugin struct {
	m7s.Plugin
	pb.UnimplementedApiServer
}

var _ = m7s.InstallPlugin[RTPPlugin](
	m7s.PluginMeta{
		ServiceDesc:         &pb.Api_ServiceDesc,
		RegisterGRPCHandler: pb.RegisterApiHandler,
	},
)

func (p *RTPPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/replay/ps/{streamPath...}": p.api_ps_replay,
	}
}

func (p *RTPPlugin) api_ps_replay(w http.ResponseWriter, r *http.Request) {
	dump := r.URL.Query().Get("dump")
	streamPath := r.PathValue("streamPath")
	if dump == "" {
		dump = "dump/ps"
	}
	if streamPath == "" {
		if strings.HasPrefix(dump, "/") {
			streamPath = "replay" + dump
		} else {
			streamPath = "replay/" + dump
		}
	}
	var puller mrtp.DumpPuller
	puller.GetPullJob().Init(&puller, &p.Plugin, streamPath, config.Pull{
		URL: dump,
	}, nil)
}

func (p *RTPPlugin) ReceivePS(ctx context.Context, req *pb.ReceivePSRequest) (resp *pb.ReceivePSResponse, err error) {
	resp = &pb.ReceivePSResponse{}
	// 获取媒体信息
	mediaPort := uint16(req.Port)
	if mediaPort == 0 {
		if req.Udp {
			// TODO: udp sppport
			resp.Code = 501
			return resp, fmt.Errorf("udp not supported")
		}
		// if gb.MediaPort.Valid() {
		// 	select {
		// 	case mediaPort = <-gb.tcpPorts:
		// 		defer func() {
		// 			if receiver != nil {
		// 				receiver.OnDispose(func() {
		// 					gb.tcpPorts <- mediaPort
		// 				})
		// 			}
		// 		}()
		// 	default:
		// 		resp.Code = 500
		// 		resp.Message = "没有可用的媒体端口"
		// 		return resp, fmt.Errorf("没有可用的媒体端口")
		// 	}
		// } else {
		// 	mediaPort = gb.MediaPort[0]
		// }
	}
	receiver := &mrtp.PSReceiver{}
	receiver.ListenAddr = fmt.Sprintf(":%d", mediaPort)
	receiver.StreamMode = mrtp.StreamModeTCPPassive
	receiver.Publisher, err = p.Publish(p, req.StreamPath)
	if err != nil {
		resp.Code = 500
		resp.Message = fmt.Sprintf("发布失败: %v", err)
		p.Error("publish stream for rtp", "error", err, "streamPath", req.StreamPath)
		return resp, err
	}
	go p.RunTask(receiver)
	resp.Code = 0
	resp.Data = int32(mediaPort)
	resp.Message = "success"
	return
}

func (p *RTPPlugin) SendPS(ctx context.Context, req *pb.SendPSRequest) (*pb.SendPSResponse, error) {
	resp := &pb.SendPSResponse{}

	// 参数校验
	if req.StreamPath == "" {
		resp.Code = 400
		resp.Message = "流路径不能为空"
		return resp, nil
	}
	suber, err := p.Subscribe(ctx, req.StreamPath)
	if err != nil {
		p.Error("subscribe stream to send rtp", "error", err)
		resp.Code = 404
		resp.Message = "未找到对应的订阅"
		return resp, nil
	}

	var w io.WriteCloser
	var writeRTP func() error
	var mem util.RecyclableMemory
	allocator := util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	mem.SetAllocator(allocator)
	defer allocator.Recycle()
	var headerBuf [14]byte
	writeBuffer := make(net.Buffers, 1)
	var totalBytesSent int
	var packet rtp.Packet
	packet.Version = 2
	packet.SSRC = req.Ssrc
	packet.PayloadType = 96
	defer func() {
		p.Info("send rtp", "total", packet.SequenceNumber, "totalBytesSent", totalBytesSent)
	}()
	if req.Udp {
		conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
			IP:   net.ParseIP(req.Ip),
			Port: int(req.Port),
		})
		if err != nil {
			resp.Code = 500
			resp.Message = "连接失败"
			return resp, err
		}
		w = conn
		writeRTP = func() (err error) {
			defer mem.Recycle()
			r := mem.NewReader()
			packet.Timestamp = uint32(time.Now().UnixMilli()) * 90
			for r.Length > 0 {
				packet.SequenceNumber += 1
				buf := writeBuffer
				buf[0] = headerBuf[:12]
				_, err = packet.Header.MarshalTo(headerBuf[:12])
				if err != nil {
					return
				}
				r.RangeN(mrtp.MTUSize, func(b []byte) {
					buf = append(buf, b)
				})
				n, _ := buf.WriteTo(w)
				totalBytesSent += int(n)
			}
			return
		}
	} else {
		p.Info("connect tcp to send rtp", "ip", req.Ip, "port", req.Port)
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   net.ParseIP(req.Ip),
			Port: int(req.Port),
		})
		if err != nil {
			p.Error("connect tcp to send rtp", "error", err)
			resp.Code = 500
			resp.Message = "连接失败"
			return resp, err
		}
		w = conn
		writeRTP = func() (err error) {
			defer mem.Recycle()
			r := mem.NewReader()
			packet.Timestamp = uint32(time.Now().UnixMilli()) * 90

			// 检查是否需要分割成多个RTP包
			const maxRTPSize = 65535 - 12 // uint16最大值减去RTP头部长度

			for r.Length > 0 {
				buf := writeBuffer
				buf[0] = headerBuf[:14]
				packet.SequenceNumber += 1

				// 计算当前包的有效载荷大小
				payloadSize := r.Length
				if payloadSize > maxRTPSize {
					payloadSize = maxRTPSize
				}

				// 设置TCP长度字段 (2字节) + RTP头部长度 (12字节) + 载荷长度
				rtpPacketSize := uint16(12 + payloadSize)
				binary.BigEndian.PutUint16(headerBuf[:2], rtpPacketSize)

				// 生成RTP头部
				_, err = packet.Header.MarshalTo(headerBuf[2:14])
				if err != nil {
					return
				}

				// 添加载荷数据
				r.RangeN(payloadSize, func(b []byte) {
					buf = append(buf, b)
				})

				// 发送RTP包
				n, writeErr := buf.WriteTo(w)
				if writeErr != nil {
					return writeErr
				}
				totalBytesSent += int(n)
			}
			return
		}
	}
	defer w.Close()
	var muxer mpegps.MpegPSMuxer
	muxer.Subscriber = suber
	muxer.Packet = &mem
	muxer.Mux(writeRTP)
	return resp, nil
}

func (p *RTPPlugin) Forward(ctx context.Context, req *pb.ForwardRequest) (res *pb.ForwardResponse, err error) {
	res = &pb.ForwardResponse{}

	// 创建转发配置
	config := &mrtp.ForwardConfig{
		Source: mrtp.ConnectionConfig{
			IP:   req.Source.Ip,
			Port: req.Source.Port,
			Mode: mrtp.StreamMode(req.Source.Mode),
			SSRC: req.Source.Ssrc,
		},
		Target: mrtp.ConnectionConfig{
			IP:   req.Target.Ip,
			Port: req.Target.Port,
			Mode: mrtp.StreamMode(req.Target.Mode),
			SSRC: req.Target.Ssrc,
		},
		Relay: req.Target.Ssrc == 0 && req.Source.Ssrc == 0,
	}

	// 创建转发器
	forwarder := mrtp.NewForwarder(config)

	// 执行转发
	err = forwarder.Forward(ctx)
	if err != nil {
		p.Error("forward failed", "error", err)
		res.Code = 500
		res.Message = fmt.Sprintf("forward failed: %v", err)
		return res, nil
	}

	res.Success = true
	res.Code = 0
	res.Message = "success"
	return res, nil
}
