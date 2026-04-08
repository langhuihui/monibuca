package rtsp

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"net"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"m7s.live/v5/pkg"

	"github.com/langhuihui/gomem"
	task "github.com/langhuihui/gotask"
	"m7s.live/v5"
	"m7s.live/v5/pkg/util"
)

const Timeout = time.Second * 30

func NewNetConnection(conn net.Conn) *NetConnection {
	c := &NetConnection{
		Conn:            conn,
		BufReader:       util.NewBufReader(conn),
		MemoryAllocator: gomem.NewScalableMemoryAllocator(1 << 22), // 4MB 起始，避免多次 children 扩容
		UserAgent:       "monibuca" + m7s.Version,
	}
	c.BufReader.SetTimeout(Timeout)
	return c
}

type NetConnection struct {
	task.Job
	*util.BufReader
	Backchannel     bool
	Media           string
	PacketSize      uint16
	SessionName     string
	Timeout         int
	Transport       string // custom transport support, ex. RTSP over WebSocket
	MemoryAllocator *gomem.ScalableMemoryAllocator
	UserAgent       string
	URL             *url.URL

	// internal

	Auth        *util.Auth
	Conn        net.Conn
	keepalive   int
	sequence    int
	Session     string
	sdp         string
	writing     atomic.Bool
	SDP         string
	keepaliveTS time.Time
}

func (c *NetConnection) StartWrite() {
	for !c.writing.CompareAndSwap(false, true) {
		runtime.Gosched()
	}
}

func (c *NetConnection) StopWrite() {
	c.writing.Store(false)
}

func (c *NetConnection) Dispose() {
	if c.Conn != nil {
		c.Conn.Close()
	}
	if c.BufReader != nil {
		c.BufReader.Recycle()
	}
	if c.MemoryAllocator != nil {
		c.MemoryAllocator.Recycle()
	}
	c.Info("destroy connection")
}

const (
	ProtoRTSP      = "RTSP/1.0"
	MethodOptions  = "OPTIONS"
	MethodSetup    = "SETUP"
	MethodTeardown = "TEARDOWN"
	MethodDescribe = "DESCRIBE"
	MethodPlay     = "PLAY"
	MethodPause    = "PAUSE"
	MethodAnnounce = "ANNOUNCE"
	MethodRecord   = "RECORD"
)

type State byte

func (s State) String() string {
	switch s {
	case StateNone:
		return "NONE"
	case StateConn:

		return "CONN"
	case StateSetup:
		return MethodSetup
	case StatePlay:
		return MethodPlay
	}
	return strconv.Itoa(int(s))
}

const (
	StateNone State = iota
	StateConn
	StateSetup
	StatePlay
)

func (c *NetConnection) Connect(ctx context.Context, remoteURL string) (err error) {
	rtspURL, err := url.Parse(remoteURL)
	if err != nil {
		return
	}
	istls := rtspURL.Scheme == "rtsps"
	if strings.Count(rtspURL.Host, ":") == 0 {
		if istls {
			rtspURL.Host += ":443"
		} else {
			rtspURL.Host += ":554"
		}
	}
	var conn net.Conn
	dialer := &net.Dialer{Timeout: Timeout}
	if istls {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config:    &tls.Config{InsecureSkipVerify: true},
		}
		conn, err = tlsDialer.DialContext(ctx, "tcp", rtspURL.Host)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", rtspURL.Host)
	}
	if err != nil {
		return
	}
	c.Conn = conn
	c.BufReader = util.NewBufReader(conn)
	c.BufReader.SetTimeout(Timeout)
	c.UserAgent = "monibuca" + m7s.Version
	c.Session = ""
	c.Auth = util.NewAuth(rtspURL.User)
	c.URL = rtspURL
	c.URL.User = nil
	c.SetDescription("remoteAddr", conn.RemoteAddr().String())
	if c.MemoryAllocator != nil {
		// 重连时旧 allocator 可能还有 in-flight 帧引用，不能立即 Recycle，
		// 让其自然被 GC 回收；但创建新 allocator 前先置 nil 断开引用。
		c.MemoryAllocator = nil
	}
	c.MemoryAllocator = gomem.NewScalableMemoryAllocator(1 << 22) // 从 4KB 改为 4MB，避免多次 children 扩容
	// c.Backchannel = true
	return
}

func (c *NetConnection) WriteRequest(req *util.Request) (err error) {
	if req.Proto == "" {
		req.Proto = ProtoRTSP
	}

	if req.Header == nil {
		req.Header = make(map[string][]string)
	}

	c.sequence++
	// important to send case sensitive CSeq
	// https://github.com/AlexxIT/go2rtc/issues/7
	req.Header["CSeq"] = []string{strconv.Itoa(c.sequence)}

	c.Auth.Write(req)

	if c.Session != "" {
		req.Header.Set("Session", c.Session)
	}

	if req.Body != nil {
		val := strconv.Itoa(len(req.Body))
		req.Header.Set("Content-Length", val)
	}

	if err = c.Conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}
	reqStr := req.String()
	c.Debug("->", "req", reqStr)
	_, err = c.Conn.Write([]byte(reqStr))
	return
}

func (c *NetConnection) ReadRequest() (req *util.Request, err error) {
	req, err = util.ReadRequest(c.BufReader)
	if err != nil {
		return
	}
	c.SetDescription("lastReq", req.Method)
	c.Debug("<-", "req", req.String())
	return
}

func (c *NetConnection) WriteResponse(res *util.Response) (err error) {
	if res.Proto == "" {
		res.Proto = ProtoRTSP
	}

	if res.StatusCode == 0 && res.Status == "" {
		res.SetStatus(200, "OK")
	}

	if res.Header == nil {
		res.Header = make(map[string][]string)
	}

	if res.Request != nil && res.Request.Header != nil {
		seq := res.Request.Header.Get("CSeq")
		if seq != "" {
			res.Header.Set("CSeq", seq)
		}
	}

	if c.Session != "" {
		if res.Request != nil && res.Request.Method == MethodSetup {
			res.Header.Set("Session", c.Session+";timeout=60")
		} else {
			res.Header.Set("Session", c.Session)
		}
	}

	if res.Body != nil {
		val := strconv.Itoa(len(res.Body))
		res.Header.Set("Content-Length", val)
	}

	if err = c.Conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}
	resStr := res.String()
	if res.Request != nil {
		c.SetDescription("lastRes", res.Request.Method)
	}
	c.Debug("->", "res", resStr)
	_, err = c.Conn.Write([]byte(resStr))
	return
}

func (c *NetConnection) ReadResponse() (res *util.Response, err error) {
	res, err = util.ReadResponse(c.BufReader)
	if err == nil {
		c.Debug("<-", "res", res.String())
	}
	return
}

func (c *NetConnection) Receive(sendMode bool, onReceive func(byte, []byte) error, onRTCP func(byte, []byte) error) (err error) {
	for err == nil {
		if err = c.StopReason(); err != nil {
			return
		}
		ts := time.Now()

		var magic []byte
		// we can read:
		// 1. RTP interleaved: `$` + 1B channel number + 2B size
		// 2. RTSP response:   RTSP/1.0 200 OK
		// 3. RTSP request:    OPTIONS ...

		if magic, err = c.Peek(4); err != nil {
			return
		}

		var size int
		if magic[0] != '$' {
			magicWord := string(magic)
			c.Warn("not magic", "magic", magicWord)
			switch magicWord {
			case "RTSP":
				var res *util.Response
				if res, err = c.ReadResponse(); err != nil {
					return
				}
				c.Warn("response", "res", res.String())
				// for playing backchannel only after OK response on play

				continue

			case "OPTI", "TEAR", "DESC", "SETU", "PLAY", "PAUS", "RECO", "ANNO", "GET_", "SET_":
				var req *util.Request
				if req, err = c.ReadRequest(); err != nil {
					return
				}

				if req.Method == MethodOptions {
					res := &util.Response{Request: req}
					if sendMode {
						c.StartWrite()
					}
					if err = c.WriteResponse(res); err != nil {
						return
					}
					if sendMode {
						c.StopWrite()
					}
				}
				continue

			default:
				c.Error("wrong input")
				//c.Fire("RTSP wrong input")
				//
				//for i := 0; ; i++ {
				//	// search next start symbol
				//	if _, err = c.reader.ReadBytes('$'); err != nil {
				//		return err
				//	}
				//
				//	if channelID, err = c.reader.ReadByte(); err != nil {
				//		return err
				//	}
				//
				//	// TODO: better check maximum good channel ID
				//	if channelID >= 20 {
				//		continue
				//	}
				//
				//	buf4 = make([]byte, 2)
				//	if _, err = io.ReadFull(c.reader, buf4); err != nil {
				//		return err
				//	}
				//
				//	// check if size good for RTP
				//	size = binary.BigEndian.Uint16(buf4)
				//	if size <= 1500 {
				//		break
				//	}
				//
				//	// 10 tries to find good packet
				//	if i >= 10 {
				//		return fmt.Errorf("RTSP wrong input")
				//	}
				//}
				for err = c.Skip(1); err == nil; {
					if magic[0], err = c.ReadByte(); magic[0] == '*' {
						var channelID byte
						channelID, err = c.ReadByte()
						magic[2], err = c.ReadByte()
						magic[3], err = c.ReadByte()
						size = int(binary.BigEndian.Uint16(magic[2:]))
						// 使用 make 而非 SMA Malloc，避免音视频包交错导致 SMA 碎片化
						buf := make([]byte, size)
						if err = c.ReadNto(size, buf); err != nil {
							return
						} else if onReceive != nil {
							if recvErr := onReceive(channelID, buf); recvErr == nil {
								// 内存被接管，不需要释放
							} else if errors.Is(recvErr, pkg.ErrDiscard) || errors.Is(recvErr, pkg.ErrMuted) {
								// 丢弃错误和静音错误，继续循环（make buf 由 GC 回收）
							} else {
								// 其他错误，终止循环
								return recvErr
							}
						}
						break
					}
				}
			}
		} else {
			// hope that the odd channels are always RTCP
			channelID := magic[1]

			// get data size
			size = int(binary.BigEndian.Uint16(magic[2:]))
			// skip 4 bytes from c.reader.Peek
			if err = c.Skip(4); err != nil {
				return
			}
			// 使用 make 而非 SMA Malloc，避免音视频包交错导致 SMA 碎片化；
			// GC 负责回收未被 AddRecycleBytes 接管的短生命周期包。
			buf := make([]byte, size)
			if err = c.ReadNto(size, buf); err != nil {
				return
			}

			if channelID&1 == 0 { // 偶数通道，RTP数据
				if onReceive != nil {
					if recvErr := onReceive(channelID, buf); recvErr == nil {
						// 内存被接管（AddRecycleBytes），不需要额外释放
					} else if errors.Is(recvErr, pkg.ErrDiscard) || errors.Is(recvErr, pkg.ErrMuted) {
						// 丢弃/静音，buf 由 GC 回收
					} else {
						// 其他错误，终止循环
						return recvErr
					}
				}
			} else if onRTCP != nil { // 奇数通道，RTCP数据
				onRTCP(channelID, buf) // buf 由 GC 回收
			}
		}

		if ts.After(c.keepaliveTS) {
			req := &util.Request{Method: MethodOptions, URL: c.URL}
			if err = c.WriteRequest(req); err != nil {
				return
			}
			c.keepaliveTS = ts.Add(25 * time.Second)
		}
	}
	return
}

func (c *NetConnection) Write(chunk []byte) (int, error) {
	if err := c.Conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Write(chunk)
}
