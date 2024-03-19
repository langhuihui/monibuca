package config

import (
	"regexp"

	"gopkg.in/yaml.v3"
)

type Regexp struct {
	*regexp.Regexp
}

func (r *Regexp) Valid() bool {
	return r.Regexp != nil
}

func (r Regexp) String() string {
	if !r.Valid() {
		return ""
	}
	return r.Regexp.String()
}

func (r *Regexp) UnmarshalYAML(node *yaml.Node) error {
	r.Regexp = regexp.MustCompile(node.Value)
	return nil
}

func (r Regexp) MarshalYAML() (interface{}, error) {
	return r.String(), nil
}

func (r Regexp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + r.String() + `"`), nil
}

func (r *Regexp) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if b[0] == '"' {
		b = b[1:]
	}
	if b[len(b)-1] == '"' {
		b = b[:len(b)-1]
	}
	r.Regexp = regexp.MustCompile(string(b))
	return nil
}
