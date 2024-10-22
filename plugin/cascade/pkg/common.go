package cascade

import (
	"m7s.live/pro/pkg/util"
)

var ENDFLAG = []byte{0}

type Superior struct {
}

var SubordinateMap util.Collection[uint, *Instance]
