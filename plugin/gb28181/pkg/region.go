package gb28181

import (
	"strconv"
	"time"
)

// Region 区域信息
type Region struct {
	ID             int    `gorm:"primaryKey;autoIncrement" json:"id"`            // 数据库自增ID
	DeviceID       string `gorm:"column:device_id" json:"deviceId"`              // 区域国标编号
	Name           string `gorm:"column:name" json:"name"`                       // 区域名称
	ParentID       int    `gorm:"column:parent_id" json:"parentId"`              // 父区域ID
	ParentDeviceID string `gorm:"column:parent_device_id" json:"parentDeviceId"` // 父区域国标ID
	BusinessGroup  string `gorm:"column:business_group" json:"businessGroup"`    // 所属的业务分组国标编号
	CreateTime     string `gorm:"column:create_time" json:"createTime"`          // 创建时间
	UpdateTime     string `gorm:"column:update_time" json:"updateTime"`          // 更新时间
	CivilCode      string `gorm:"column:civil_code" json:"civilCode"`            // 行政区划
}

// TableName 指定数据库表名
func (r *Region) TableName() string {
	return "region_gb28181pro"
}

// NewRegion 创建新的区域实例
func NewRegion(deviceID, name, parentDeviceID string) *Region {
	now := time.Now().Format("2006-01-02 15:04:05")
	return &Region{
		DeviceID:       deviceID,
		Name:           name,
		ParentDeviceID: parentDeviceID,
		CreateTime:     now,
		UpdateTime:     now,
	}
}

// NewRegionFromCivilCode 从行政区划编码创建区域实例
func NewRegionFromCivilCode(civilCode *CivilCode) *Region {
	if civilCode == nil {
		return nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	region := &Region{
		Name:       civilCode.Name,
		DeviceID:   civilCode.Code,
		CreateTime: now,
		UpdateTime: now,
	}

	// 如果编码长度大于2，设置父级编码
	if len(civilCode.Code) > 2 {
		region.ParentDeviceID = civilCode.ParentCode
	}

	return region
}

// NewRegionFromChannel 从设备通道创建区域实例
func NewRegionFromChannel(channel *DeviceChannel) *Region {
	if channel == nil {
		return nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	region := &Region{
		Name:       channel.Name,
		DeviceID:   channel.DeviceId,
		CreateTime: now,
		UpdateTime: now,
	}

	// 获取父级编码
	parentCode := GetInstance().GetParentCode(channel.DeviceId)
	if parentCode != nil {
		region.ParentDeviceID = parentCode.Code
	}

	return region
}

// CompareTo 实现比较功能
func (r *Region) CompareTo(other *Region) int {
	thisID, _ := strconv.Atoi(r.DeviceID)
	otherID, _ := strconv.Atoi(other.DeviceID)
	return thisID - otherID
}

// Equals 判断两个区域是否相等
func (r *Region) Equals(other interface{}) bool {
	if other == nil {
		return false
	}
	if r == other {
		return true
	}
	otherRegion, ok := other.(*Region)
	if !ok {
		return false
	}
	return r.ID == otherRegion.ID
}

// GetParentCode 获取父级编码（这个函数需要在 civilcodeutil.go 中实现）
func GetParentCode(deviceID string) *CivilCode {
	// TODO: 实现获取父级编码的逻辑
	// 这部分需要参考 CivilCodeUtil.java 的实现
	return nil
}
