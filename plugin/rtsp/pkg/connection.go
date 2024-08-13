package rtsp

import (
	"encoding/binary"
	"net"
	"net/url"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
)

const Timeout = time.Second * 5

func NewNetConnection(conn net.Conn) *NetConnection {
	return &NetConnection{
		conn:            conn,
		BufReader:       util.NewBufReader(conn),
		MemoryAllocator: util.NewScalableMemoryAllocator(1 << 12),
		UserAgent:       "monibuca" + m7s.Version,
	}
}

type NetConnection struct {
	util.MarcoTask
	*util.BufReader
	Backchannel     bool
	Media           string
	PacketSize      uint16
	SessionName     string
	Timeout         int
	Transport       string // custom transport support, ex. RTSP over WebSocket
	MemoryAllocator *util.ScalableMemoryAllocator
	UserAgent       string
	URL             *url.URL

	// internal

	auth      *util.Auth
	conn      net.Conn
	keepalive int
	sequence  int
	Session   string
	sdp       string
	uri       string
	writing   atomic.Bool
	state     State
	stateMu   sync.Mutex
	SDP       string
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
	c.conn.Close()
	c.BufReader.Recycle()
	c.MemoryAllocator.Recycle()
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

	c.auth.Write(req)

	if c.Session != "" {
		req.Header.Set("Session", c.Session)
	}

	if req.Body != nil {
		val := strconv.Itoa(len(req.Body))
		req.Header.Set("Content-Length", val)
	}

	if err = c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}
	reqStr := req.String()
	c.Debug("->", "req", reqStr)
	_, err = c.conn.Write([]byte(reqStr))
	return
}

func (c *NetConnection) ReadRequest() (req *util.Request, err error) {
	if err = c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return
	}
	req, err = util.ReadRequest(c.BufReader)
	if err != nil {
		return
	}
	c.Debug("<-", "req", req.String())
	return
}

func (c *NetConnection) WriteResponse(res *util.Response) (err error) {
	if res.Proto == "" {
		res.Proto = ProtoRTSP
	}

	if res.Status == "" {
		res.Status = "200 OK"
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

	if err = c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}
	resStr := res.String()
	c.Debug("->", "res", resStr)
	_, err = c.conn.Write([]byte(resStr))
	return
}

func (c *NetConnection) ReadResponse() (res *util.Response, err error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return nil, err
	}
	res, err = util.ReadResponse(c.BufReader)
	if err == nil {
		c.Debug("<-", "res", res.String())
	}
	return
}

func (c *NetConnection) Receive(sendMode bool) (channelID byte, buf []byte, err error) {
	ts := time.Now()
	if err = c.conn.SetReadDeadline(ts.Add(util.Conditoinal(sendMode, time.Second*60, time.Second*15))); err != nil {
		return
	}
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
			c.Warn(string(res.Body))
			// for playing backchannel only after OK response on play

			return

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
			return

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
					channelID, err = c.ReadByte()
					magic[2], err = c.ReadByte()
					magic[3], err = c.ReadByte()
					size = int(binary.BigEndian.Uint16(magic[2:]))
					buf = c.MemoryAllocator.Malloc(size)
					if err = c.ReadNto(size, buf); err != nil {
						return
					}
					break
				}
			}
		}
	} else {
		// hope that the odd channels are always RTCP

		channelID = magic[1]

		// get data size
		size = int(binary.BigEndian.Uint16(magic[2:]))
		// skip 4 bytes from c.reader.Peek
		if err = c.Skip(4); err != nil {
			return
		}
		buf = c.MemoryAllocator.Malloc(size)
		if err = c.ReadNto(size, buf); err != nil {
			c.MemoryAllocator.Free(buf)
			return
		}
	}

	//if keepaliveDT != 0 && ts.After(keepaliveTS) {
	//	req := &tcp.Request{Method: MethodOptions, URL: c.URL}
	//	if err = c.WriteRequest(req); err != nil {
	//		return
	//	}
	//
	//	keepaliveTS = ts.Add(keepaliveDT)
	//}
	return
}

func (c *NetConnection) Write(chunk []byte) (int, error) {
	if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return 0, err
	}
	return c.conn.Write(chunk)
}
