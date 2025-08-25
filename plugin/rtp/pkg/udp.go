package rtp

import (
	"net"

	"m7s.live/v5/pkg/util"
)

type UDP net.UDPConn

func (t *UDP) Read(onRTP func(util.Buffer) error) (err error) {
	buffer := make(util.Buffer, 1024*1024)

	for {
		n, _, err := (*net.UDPConn)(t).ReadFromUDP(buffer)
		if err != nil {
			return err
		}

		err = onRTP(buffer[:n])
		if err != nil {
			//return err
		}
	}
}
