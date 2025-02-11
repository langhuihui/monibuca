package plugin_gb28181pro

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/emiago/sipgo"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

type Dialog struct {
	task.Job
	Channel *Channel
	gb28181.InviteOptions
	gb      *GB28181ProPlugin
	session *sipgo.DialogClientSession
	pullCtx m7s.PullJob
}

func (d *Dialog) GetCallID() string {
	return d.session.InviteRequest.CallID().Value()
}

func (d *Dialog) GetPullJob() *m7s.PullJob {
	return &d.pullCtx
}

func (d *Dialog) Start() (err error) {
	if !d.IsLive() {
		d.pullCtx.PublishConfig.PubType = m7s.PublishTypeVod
	}
	err = d.pullCtx.Publish()
	if err != nil {
		return
	}
	sss := strings.Split(d.pullCtx.RemoteURL, "/")
	deviceId, channelId := sss[0], sss[1]
	if len(sss) == 2 {
		if device, ok := d.gb.devices.Get(deviceId); ok {
			if channel, ok := device.channels.Get(channelId); ok {
				d.Channel = channel
			} else {
				return fmt.Errorf("channel %s not found", channelId)
			}
		} else {
			return fmt.Errorf("device %s not found", deviceId)
		}
	} else if len(sss) == 3 {
		var recordRange util.Range[int]
		err = recordRange.Resolve(sss[2])
	}

	d.gb.dialogs.Set(d)
	defer d.gb.dialogs.Remove(d)
	if d.gb.MediaPort.Valid() {
		select {
		case d.MediaPort = <-d.gb.tcpPorts:
			defer func() {
				d.gb.tcpPorts <- d.MediaPort
			}()
		default:
			return fmt.Errorf("no available tcp port")
		}
	} else {
		d.MediaPort = d.gb.MediaPort[0]
	}

	// 调用 PlayStreamCmd
	d.session, err = d.gb.PlayStreamCmd(d.Channel.Device, d.Channel, d.MediaPort)
	if err != nil {
		return fmt.Errorf("play stream failed: %v", err)
	}

	return
}

func (d *Dialog) Run() (err error) {
	err = d.session.WaitAnswer(d.gb, sipgo.AnswerOptions{})
	if err != nil {
		return
	}
	inviteResponseBody := string(d.session.InviteResponse.Body())
	d.Channel.Info("inviteResponse", "body", inviteResponseBody)
	ds := strings.Split(inviteResponseBody, "\r\n")
	for _, l := range ds {
		if ls := strings.Split(l, "="); len(ls) > 1 {
			if ls[0] == "y" && len(ls[1]) > 0 {
				if _ssrc, err := strconv.ParseInt(ls[1], 10, 0); err == nil {
					d.SSRC = uint32(_ssrc)
				} else {
					d.gb.Error("read invite response y ", "err", err)
				}
				//	break
			}
			if ls[0] == "m" && len(ls[1]) > 0 {
				netinfo := strings.Split(ls[1], " ")
				if strings.ToUpper(netinfo[2]) == "TCP/RTP/AVP" {
					d.gb.Debug("device support tcp")
				} else {
					return fmt.Errorf("device not support tcp")
				}
			}
		}
	}
	err = d.session.Ack(d.gb)
	pub := gb28181.NewPSPublisher(d.pullCtx.Publisher)
	pub.Receiver.ListenAddr = fmt.Sprintf(":%d", d.MediaPort)
	pub.Receiver.ListenPort = d.MediaPort
	d.AddTask(&pub.Receiver)
	pub.Demux()
	return
}

func (d *Dialog) GetKey() uint32 {
	return d.SSRC
}

func (d *Dialog) Dispose() {
	d.session.Close()
}
