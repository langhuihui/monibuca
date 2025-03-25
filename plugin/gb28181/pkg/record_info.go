package gb28181

import "time"

// RecordInfo 设备录像信息
type RecordInfo struct {
	// 设备编号
	DeviceID string `json:"deviceId"`

	// 通道编号
	ChannelID string `json:"channelId"`

	// 命令序列号
	SN string `json:"sn"`

	// 设备名称
	Name string `json:"name"`

	// 列表总数
	SumNum int `json:"sumNum"`

	// 计数
	Count int `json:"count"`

	// 最后时间
	LastTime time.Time `json:"lastTime"`

	// 录像列表
	RecordList []RecordItem `json:"recordList"`
}

// NewRecordInfo 创建新的 RecordInfo 实例
func NewRecordInfo() *RecordInfo {
	return &RecordInfo{
		RecordList: make([]RecordItem, 0),
	}
}

// GetDeviceID 获取设备编号
func (r *RecordInfo) GetDeviceID() string {
	return r.DeviceID
}

// SetDeviceID 设置设备编号
func (r *RecordInfo) SetDeviceID(deviceID string) {
	r.DeviceID = deviceID
}

// GetName 获取设备名称
func (r *RecordInfo) GetName() string {
	return r.Name
}

// SetName 设置设备名称
func (r *RecordInfo) SetName(name string) {
	r.Name = name
}

// GetSumNum 获取列表总数
func (r *RecordInfo) GetSumNum() int {
	return r.SumNum
}

// SetSumNum 设置列表总数
func (r *RecordInfo) SetSumNum(sumNum int) {
	r.SumNum = sumNum
}

// GetRecordList 获取录像列表
func (r *RecordInfo) GetRecordList() []RecordItem {
	return r.RecordList
}

// SetRecordList 设置录像列表
func (r *RecordInfo) SetRecordList(recordList []RecordItem) {
	r.RecordList = recordList
}

// GetChannelID 获取通道编号
func (r *RecordInfo) GetChannelID() string {
	return r.ChannelID
}

// SetChannelID 设置通道编号
func (r *RecordInfo) SetChannelID(channelID string) {
	r.ChannelID = channelID
}

// GetSN 获取命令序列号
func (r *RecordInfo) GetSN() string {
	return r.SN
}

// SetSN 设置命令序列号
func (r *RecordInfo) SetSN(sn string) {
	r.SN = sn
}

// GetLastTime 获取最后时间
func (r *RecordInfo) GetLastTime() time.Time {
	return r.LastTime
}

// SetLastTime 设置最后时间
func (r *RecordInfo) SetLastTime(lastTime time.Time) {
	r.LastTime = lastTime
}

// GetCount 获取计数
func (r *RecordInfo) GetCount() int {
	return r.Count
}

// SetCount 设置计数
func (r *RecordInfo) SetCount(count int) {
	r.Count = count
}
