package gb28181

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
)

type InviteOptions struct {
	Start       string
	End         string
	dump        string
	ssrc        string
	SSRC        uint32
	MediaPort   uint16
	StreamPath  string
	recyclePort func(p uint16) (err error)
}

func (o InviteOptions) IsLive() bool {
	return o.Start == "" && o.End == ""
}

func (o InviteOptions) Record() bool {
	return !o.IsLive()
}

func (o *InviteOptions) Validate(start, end string) error {
	var sint int64
	var eint int64
	if start != "" {
		sinttmp, err1 := strconv.ParseInt(start, 10, 0)
		if err1 != nil {
			return err1
		}
		sint = sinttmp
		o.Start = start
	}
	if end != "" {
		einttmp, err2 := strconv.ParseInt(end, 10, 0)
		if err2 != nil {
			return err2
		}
		eint = einttmp
		o.End = end
	}
	if sint >= eint {
		return errors.New("start < end")
	}
	return nil
}

func (o InviteOptions) String() string {
	return fmt.Sprintf("t=%s %s", o.Start, o.End)
}

func (o *InviteOptions) CreateSSRC(serial string) string {
	ssrc := make([]byte, 10)
	if o.IsLive() {
		ssrc[0] = '0'
	} else {
		ssrc[0] = '1'
	}
	copy(ssrc[1:6], serial[3:8])
	randNum := 1000 + rand.Intn(8999)
	copy(ssrc[6:], strconv.Itoa(randNum))
	o.ssrc = string(ssrc)
	_ssrc, _ := strconv.ParseInt(o.ssrc, 10, 0)
	o.SSRC = uint32(_ssrc)
	return o.ssrc
}
