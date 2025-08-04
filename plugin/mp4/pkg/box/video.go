package box

import "m7s.live/v5/pkg/util"

type Sample struct {
	util.Memory
	KeyFrame  bool
	Timestamp uint32
	CTS       uint32
	Offset    int64
	Duration  uint32
}
