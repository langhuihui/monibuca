package plugin_gb28181

import (
	"log/slog"
	"m7s.live/m7s/v5/pkg/util"
	gb28181 "m7s.live/m7s/v5/plugin/gb28181/pkg"
	"sync/atomic"
	"time"
)

type RecordRequest struct {
	SN, SumNum int
	*util.Promise[[]gb28181.Record]
}

func (r *RecordRequest) GetKey() int {
	return r.SN
}

type Channel struct {
	Device              *Device      // 所属设备
	State               atomic.Int32 // 通道状态,0:空闲,1:正在invite,2:正在播放/对讲
	GpsTime             time.Time    // gps时间
	Longitude, Latitude string       // 经度
	RecordReqs          util.Collection[int, *RecordRequest]
	*slog.Logger
	gb28181.ChannelInfo
}

func (c *Channel) GetKey() string {
	return c.DeviceID
}
