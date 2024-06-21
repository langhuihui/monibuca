package rtsp

import (
	"bufio"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"log/slog"
	"m7s.live/m7s/v5/pkg/util"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const Timeout = time.Second * 5

func NewNetConnection(conn net.Conn, logger *slog.Logger) *NetConnection {
	defer logger.Info("new connection")
	return &NetConnection{
		conn:       conn,
		Logger:     logger,
		BufReader:  util.NewBufReader(conn),
		textReader: bufio.NewReader(conn),
	}
}

type NetConnection struct {
	*slog.Logger
	*util.BufReader
	textReader  *bufio.Reader
	Backchannel bool
	Media       string
	PacketSize  uint16
	SessionName string
	Timeout     int
	Transport   string // custom transport support, ex. RTSP over WebSocket

	Medias    []*core.Media
	UserAgent string
	URL       *url.URL

	// internal

	auth      *tcp.Auth
	conn      net.Conn
	keepalive int
	mode      core.Mode
	sequence  int
	Session   string
	sdp       string
	uri       string

	state   State
	stateMu sync.Mutex
	SDP     string
}

func (c *NetConnection) Destroy() {
	c.conn.Close()
	c.BufReader.Recycle()
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

func (c *NetConnection) WriteRequest(req *tcp.Request) error {
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

	if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}

	return req.Write(c.conn)
}

func (c *NetConnection) ReadRequest() (req *tcp.Request, err error) {
	if err = c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return
	}
	req, err = tcp.ReadRequest(c.textReader)
	if err != nil {
		return
	}
	c.Debug(req.String())
	return
}

func (c *NetConnection) WriteResponse(res *tcp.Response) error {
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

	if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}
	c.Debug(res.String())
	return res.Write(c.conn)
}

func (c *NetConnection) ReadResponse() (*tcp.Response, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return nil, err
	}
	return tcp.ReadResponse(c.textReader)
}
