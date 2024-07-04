package util

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"strconv"
	"strings"
)

type Range[T ~int | ~int8 | ~int16 | ~int32 | ~int64 |
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr] [2]T

func (r *Range[T]) Size() T {
	return r[1] - r[0]
}

func (r *Range[T]) Within(x T) bool {
	return x >= r[0] && x <= r[1]
}

func (r *Range[T]) Valid() bool {
	return r.Size() >= 0
}

func (r *Range[T]) Resolve(s string) error {
	ss := strings.Split(s, "-")
	if len(ss) == 0 {
		return fmt.Errorf("invalid range: %s", s)
	}
	if len(ss) == 1 {
		i64, err := strconv.ParseInt(s, 10, 0)
		r[0] = T(i64)
		if err != nil {
			return err
		}
		r[1] = r[0]
		return nil
	}
	i64, err := strconv.ParseInt(ss[0], 10, 0)
	if err != nil {
		return err
	}
	r[0] = T(i64)
	i64, err = strconv.ParseInt(ss[1], 10, 0)
	if err != nil {
		return err
	}
	r[1] = T(i64)
	return nil
}

func (r *Range[T]) UnmarshalYAML(value *yaml.Node) error {
	return r.Resolve(value.Value)
}
