package plugin_gb28181pro

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"os"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/rs/zerolog"
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
		// 先从内存中获取设备
		device, ok := d.gb.devices.Get(deviceId)
		if !ok && d.gb.DB != nil {
			// 如果内存中没有且数据库存在，则从数据库查询
			var dbDevice Device
			if err := d.gb.DB.Where("device_id = ?", deviceId).First(&dbDevice).Error; err == nil {
				// 恢复设备的必要字段
				dbDevice.Logger = d.gb.With("id", deviceId)
				dbDevice.channels.L = new(sync.RWMutex)
				dbDevice.plugin = d.gb
				dbDevice.eventChan = make(chan any, 10)

				// 初始化 SIP 相关字段
				dbDevice.fromHDR = sip.FromHeader{
					Address: sip.Uri{
						User: d.gb.Serial,
						Host: d.gb.Realm,
					},
					Params: sip.NewParams(),
				}
				dbDevice.fromHDR.Params.Add("tag", sip.GenerateTagN(16))

				dbDevice.contactHDR = sip.ContactHeader{
					Address: sip.Uri{
						User: d.gb.Serial,
						Host: dbDevice.LocalIP,
						Port: dbDevice.Port,
					},
				}

				dbDevice.Recipient = sip.Uri{
					Host: dbDevice.IP,
					Port: dbDevice.Port,
					User: dbDevice.DeviceID,
				}

				// 初始化 SIP 客户端
				dbDevice.client, _ = sipgo.NewClient(d.gb.ua, sipgo.WithClientLogger(zerolog.New(os.Stdout)), sipgo.WithClientHostname(dbDevice.LocalIP))
				if dbDevice.client != nil {
					dbDevice.dialogClient = sipgo.NewDialogClient(dbDevice.client, dbDevice.contactHDR)
				} else {
					return fmt.Errorf("failed to create sip client for device %s", deviceId)
				}

				device = &dbDevice
			} else {
				return fmt.Errorf("device %s not found", deviceId)
			}
		} else if !ok {
			return fmt.Errorf("device %s not found", deviceId)
		}

		// 先从内存中获取通道
		channel, ok := device.channels.Get(channelId)
		if !ok && d.gb.DB != nil {
			// 如果内存中没有且数据库存在，则从数据库查询
			var dbChannel gb28181.DeviceChannel
			if err := d.gb.DB.Where("device_id = ? AND device_db_id = ?", channelId, device.ID).First(&dbChannel).Error; err == nil {
				channel = &Channel{
					Device:        device,
					Logger:        device.Logger.With("channel", channelId),
					DeviceChannel: dbChannel,
				}
				device.channels.Set(channel)
			} else {
				return fmt.Errorf("channel %s not found", channelId)
			}
		} else if !ok {
			return fmt.Errorf("channel %s not found", channelId)
		}

		d.Channel = channel
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
	d.Channel.Info("before WaitAnswer")
	err = d.session.WaitAnswer(d.gb, sipgo.AnswerOptions{})
	d.Channel.Info("after WaitAnswer")
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
