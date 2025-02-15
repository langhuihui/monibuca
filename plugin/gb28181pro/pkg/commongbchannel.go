package gb28181

import (
	"strconv"
)

// CommonGBChannel 通用国标通道信息
type CommonGBChannel struct {
	// 流ID，存在表示正在推流
	StreamID string `json:"streamId" xml:"-"`

	// 是否含有音频
	HasAudio bool `json:"hasAudio" xml:"-"`

	// 经度
	Longitude float64 `json:"longitude" xml:"Longitude"`

	// 纬度
	Latitude float64 `json:"latitude" xml:"Latitude"`

	// 子设备数
	SubCount int `json:"subCount" xml:"-"`

	// 更新时间
	UpdateTime string `json:"updateTime" xml:"-"`

	// GPS更新时间
	GPSTime string `json:"gpsTime" xml:"-"`
}

// Build 构建通道信息
func (c *CommonGBChannel) Build(deviceID string, name string, manufacturer string, model string, owner string,
	civilCode string, block string, address string, parentID string) {
	// TODO: 实现构建逻辑
}

// GetFullContent 获取完整的通道信息内容
func (c *CommonGBChannel) GetFullContent(deviceID string, name string, parentID string, event string) string {
	content := "<Item>\n"
	content += "<DeviceID>" + deviceID + "</DeviceID>\n"
	content += "<Name>" + name + "</Name>\n"
	if parentID != "" {
		content += "<ParentID>" + parentID + "</ParentID>\n"
	}
	if c.Longitude != 0 {
		content += "<Longitude>" + strconv.FormatFloat(c.Longitude, 'f', -1, 64) + "</Longitude>\n"
	}
	if c.Latitude != 0 {
		content += "<Latitude>" + strconv.FormatFloat(c.Latitude, 'f', -1, 64) + "</Latitude>\n"
	}
	if event != "" {
		content += "<Event>" + event + "</Event>\n"
	}
	content += "</Item>\n"
	return content
}

// Encode 编码通道信息
func (c *CommonGBChannel) Encode(deviceID string, name string, parentID string, event string) string {
	if event == "" {
		return c.GetFullContent(deviceID, name, parentID, "")
	}

	switch event {
	case "DEL", "DEFECT", "VLOST", "ON", "OFF":
		return "<Item>\n" +
			"<DeviceID>" + deviceID + "</DeviceID>\n" +
			"<Event>" + event + "</Event>\n" +
			"</Item>\n"
	case "ADD", "UPDATE":
		return c.GetFullContent(deviceID, name, parentID, event)
	default:
		return ""
	}
}
