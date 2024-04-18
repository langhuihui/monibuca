package pkg

import (
	"strconv"
	"strings"
)

type (
	TCPPort      int
	UDPPort      int
	TCPRangePort [2]int
	UDPRangePort [2]int
	Port         struct {
		Protocol string
		Ports    [2]int
	}
	IPort interface {
		IsTCP() bool
		IsUDP() bool
		IsRange() bool
	}
)

func (p Port) String() string {
	if p.Ports[0] == p.Ports[1] {
		return p.Protocol + ":" + strconv.Itoa(p.Ports[0])
	}
	return p.Protocol + ":" + strconv.Itoa(p.Ports[0]) + "-" + strconv.Itoa(p.Ports[1])
}

func (p Port) IsTCP() bool {
	return p.Protocol == "tcp"
}

func (p Port) IsUDP() bool {
	return p.Protocol == "udp"
}

func (p Port) IsRange() bool {
	return p.Ports[0] != p.Ports[1]
}

func ParsePort2(conf string) (ret any, err error) {
	var port Port
	port, err = ParsePort(conf)
	if err != nil {
		return
	}
	if port.IsTCP() {
		if port.IsRange() {
			return TCPRangePort(port.Ports), nil
		}
		return TCPPort(port.Ports[0]), nil
	}
	if port.IsRange() {
		return UDPRangePort(port.Ports), nil
	}
	return UDPPort(port.Ports[0]), nil
}

func ParsePort(conf string) (ret Port, err error) {
	var port string
	var min, max int
	ret.Protocol, port, _ = strings.Cut(conf, ":")
	if r := strings.Split(port, "-"); len(r) == 2 {
		min, err = strconv.Atoi(r[0])
		if err != nil {
			return
		}
		max, err = strconv.Atoi(r[1])
		if err != nil {
			return
		}
		if min < max {
			ret.Ports[0], ret.Ports[1] = min, max
		} else {
			ret.Ports[0], ret.Ports[1] = max, min
		}
	} else if p, err := strconv.Atoi(port); err == nil {
		ret.Ports[0], ret.Ports[1] = p, p
	}
	return
}
