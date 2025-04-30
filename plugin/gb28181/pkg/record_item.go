package gb28181

import (
	"strings"
	"time"
)

// RecordItem 设备录像信息
type RecordItem struct {
	// 设备编号
	DeviceID string `xml:"DeviceId" json:"deviceId"`

	// 名称
	Name string `xml:"Name" json:"name"`

	// 文件路径名 (可选)
	FilePath string `xml:"FilePath" json:"filePath"`

	// 录像文件大小,单位:Byte(可选)
	FileSize string `xml:"FileSize" json:"fileSize"`

	// 录像地址(可选)
	Address string `xml:"Address" json:"address"`

	// 录像开始时间(可选)
	StartTime string `xml:"StartTime" json:"startTime"`

	// 录像结束时间(可选)
	EndTime string `xml:"EndTime" json:"endTime"`

	// 保密属性(必选)缺省为0;0:不涉密,1:涉密
	Secrecy int `xml:"Secrecy" json:"secrecy"`

	// 录像产生类型(可选)time或alarm或manual
	Type string `xml:"Type" json:"type"`

	// 录像触发者ID(可选)
	RecorderID string `xml:"RecorderId" json:"recorderId"`
}

// CompareTo 比较两个录像记录的开始时间
// 返回值：
// -1: r < other
//
//	0: r = other
//	1: r > other
func (r *RecordItem) CompareTo(other *RecordItem) int {
	startTimeNow, err := time.Parse("2006-01-02T15:04:05", r.StartTime)
	if err != nil {
		return 0
	}
	startTimeParam, err := time.Parse("2006-01-02T15:04:05", other.StartTime)
	if err != nil {
		return 0
	}

	if startTimeNow.Equal(startTimeParam) {
		return 0
	} else if startTimeParam.After(startTimeNow) {
		return -1
	} else {
		return 1
	}
}

// Less 用于排序
func (r *RecordItem) Less(other *RecordItem) bool {
	return r.CompareTo(other) < 0
}

// Equal 判断两个录像记录是否相等
func (r *RecordItem) Equal(other *RecordItem) bool {
	if other == nil {
		return false
	}
	return strings.EqualFold(r.StartTime, other.StartTime)
}
