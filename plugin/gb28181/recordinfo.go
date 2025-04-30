package plugin_gb28181pro

import (
	"context"
	"fmt"
	"time"

	"github.com/emiago/sipgo/sip"
	"m7s.live/v5/pkg/util"
)

// RecordInfoQuery 发送录像查询请求
// startTime 和 endTime 的格式为 "2006-01-02 15:04:05"
func (gb *GB28181Plugin) RecordInfoQuery(deviceID string, channelID string, startTime time.Time, endTime time.Time, sn int) (*util.Promise, error) {
	device, ok := gb.devices.Get(deviceID)
	if !ok {
		return nil, fmt.Errorf("device not found: %s", deviceID)
	}

	channel, ok := device.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	// 构建XML消息
	charset := "GB2312"
	if device.Charset != "" {
		charset = device.Charset
	}

	msgBody := fmt.Sprintf(`<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>RecordInfo</CmdType>
<SN>%d</SN>
<DeviceId>%s</DeviceId>
<StartTime>%s</StartTime>
<EndTime>%s</EndTime>
<Secrecy>0</Secrecy>
<Type>all</Type>
</Query>`, charset, sn, channelID, startTime.Format("2006-01-02T15:04:05"), endTime.Format("2006-01-02T15:04:05"))

	// 创建 MESSAGE 请求
	request := device.CreateRequest(sip.MESSAGE, nil)
	if request == nil {
		return nil, fmt.Errorf("create request failed")
	}

	// 设置消息体
	request.SetBody([]byte(msgBody))

	// 创建Promise并保存到channel的RecordReqs中
	promise := util.NewPromise(context.Background())
	recordReq := &RecordRequest{
		SN:      sn,
		Promise: promise,
	}

	// 先保存请求到RecordReqs，确保能接收到响应
	channel.RecordReqs.Set(recordReq)

	// 发送请求
	_, err := device.send(request)
	if err != nil {
		channel.RecordReqs.Remove(recordReq)
		return nil, err
	}

	return promise, nil
}
