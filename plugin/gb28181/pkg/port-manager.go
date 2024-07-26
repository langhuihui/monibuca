package gb28181

import (
	"errors"
)

var ErrNoAvailablePorts = errors.New("no available ports")

type PortManager struct {
	recycle chan uint16
	max     uint16
	pos     uint16
	Valid   bool
}

func (pm *PortManager) Init(start, end uint16) {
	pm.pos = start - 1
	pm.max = end
	if pm.pos > 0 && pm.max > pm.pos {
		pm.Valid = true
		pm.recycle = make(chan uint16, pm.Range())
	}
}

func (pm *PortManager) Range() uint16 {
	return pm.max - pm.pos
}

func (pm *PortManager) Recycle(p uint16) (err error) {
	select {
	case pm.recycle <- p:
		return nil
	default:
		return ErrNoAvailablePorts
	}
}

func (pm *PortManager) GetPort() (p uint16, err error) {
	select {
	case p = <-pm.recycle:
		return
	default:
		if pm.Range() > 0 {
			pm.pos++
			p = pm.pos
			return
		} else {
			return 0, ErrNoAvailablePorts
		}
	}
}
