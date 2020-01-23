package cluster

import (
	"bufio"
	"encoding/binary"
	"fmt"
	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/pool"
	"net"
	"strconv"
	"strings"
	"time"
)

func ListenBare(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if MayBeError(err) {
		return err
	}
	var tempDelay time.Duration

	for {
		conn, err := listener.Accept()
		println(conn.RemoteAddr().String())
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				println("bare: Accept error: " + err.Error() + "; retrying in " + strconv.FormatFloat(tempDelay.Seconds(), 'f', 2, 64))
				// fmt.Printf("bare: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return err
		}

		tempDelay = 0

		go process(conn)
	}
}

func process(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	stream := OutputStream{
		SendHandler: func(p *pool.SendPacket) error {
			head := pool.GetSlice(9)
			head[0] = p.Packet.Type - 7
			binary.BigEndian.PutUint32(head[1:5], p.Timestamp)
			binary.BigEndian.PutUint32(head[5:9], uint32(len(p.Packet.Payload)))
			if _, err := conn.Write(head); err != nil {
				return err
			}
			pool.RecycleSlice(head)
			if _, err := conn.Write(p.Packet.Payload); err != nil {
				return err
			}
			return nil
		}, SubscriberInfo: SubscriberInfo{
			ID:   conn.RemoteAddr().String(),
			Type: "Bare",
		},
	}
	for {
		cmd, err := reader.ReadByte()
		if err != nil {
			return
		}
		switch cmd {
		case MSG_SUBSCRIBE:
			if stream.Room != nil {
				fmt.Printf("bare stream already exist from %s", conn.RemoteAddr())
				return
			}
			bytes, err := reader.ReadBytes(0)
			if MayBeError(err) {
				return
			}
			streamName := string(bytes[0 : len(bytes)-1])
			stream.Play(streamName)
		case MSG_AUTH:
			bytes, err := reader.ReadBytes(0)
			if err != nil {
				print(err)
				return
			}
			sign := strings.Split(string(bytes[0:len(bytes)-1]), ",")
			head := []byte{MSG_AUTH, 0}
			if len(sign) > 1 && AuthHooks.Trigger(sign[1]) == nil {
				head[1] = 1
			}
			conn.Write(head)
			conn.Write(bytes)
		default:
			fmt.Printf("bare receive unknown cmd:%d from %s", cmd, conn.RemoteAddr())
			return
		}
	}
}
