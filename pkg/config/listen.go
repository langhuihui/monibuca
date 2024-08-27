package config

import (
	"gopkg.in/yaml.v3"
	"strconv"
	"strings"
)

type Listen struct {
	Protocol string
	Host     string
	Port     int
}

func (l *Listen) UnmarshalYAML(node *yaml.Node) (err error) {
	vs := strings.Split(node.Value, ":")
	vsl := len(vs)
	switch vsl {
	case 1:
		l.Protocol = "tcp"
	case 2:
		l.Protocol = vs[0]
	case 3:
		l.Protocol = vs[0]
		l.Host = vs[1]
	}
	l.Port, err = strconv.Atoi(vs[vsl-1])
	return
}
