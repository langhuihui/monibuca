package util

// DTSEstimator is a DTS estimator.
type DTSEstimator struct {
	hasB     bool
	prevPTS  uint32
	prevDTS  uint32
	cache    []uint32
	interval uint32
}

// NewDTSEstimator allocates a DTSEstimator.
func NewDTSEstimator() *DTSEstimator {
	result := &DTSEstimator{}
	return result
}

func (d *DTSEstimator) Clone() *DTSEstimator {
	return &DTSEstimator{
		d.hasB,
		d.prevPTS,
		d.prevDTS,
		append([]uint32(nil), d.cache...),
		d.interval,
	}
}

func (d *DTSEstimator) add(pts uint32) {
	i := 0
	l := len(d.cache)
	if l >= 4 {
		l--
		// i = l - 3
		d.cache = append(d.cache[:0], d.cache[1:]...)[:l]
	}

	for ; i < l; i = i + 1 {
		if d.cache[i] > pts {
			break
		}
	}
	d.cache = append(d.cache, pts)
	d.cache = append(d.cache[:i+1], d.cache[i:l]...)
	d.cache[i] = pts
}

// Feed provides PTS to the estimator, and returns the estimated DTS.
func (d *DTSEstimator) Feed(pts uint32) uint32 {
	interval := Conditoinal(pts > d.prevPTS, pts-d.prevPTS, d.prevPTS-pts)
	if interval > 10*d.interval {
		*d = *NewDTSEstimator()
	}
	d.interval = interval
	d.add(pts)
	dts := pts
	if !d.hasB {
		if pts < d.prevPTS {
			d.hasB = true
			dts = d.cache[0]
		}
	} else {
		dts = d.cache[0]
	}

	if d.prevDTS > dts {
		dts = d.prevDTS
	}
	// if d.prevDTS >= dts {
	// 	dts = d.prevDTS + 90
	// }
	d.prevPTS = pts
	d.prevDTS = dts
	return dts
}
