package gb28181

import (
	"strconv"
	"time"
)

// Group 表示业务分组
type Group struct {
	ID             int    `gorm:"primaryKey;autoIncrement" json:"id"`            // 数据库自增ID
	DeviceID       string `gorm:"column:device_id" json:"deviceId"`              // 区域国标编号
	Name           string `gorm:"column:name" json:"name"`                       // 区域名称
	ParentID       int    `gorm:"column:parent_id" json:"parentId"`              // 父分组ID
	ParentDeviceID string `gorm:"column:parent_device_id" json:"parentDeviceId"` // 父区域国标ID
	BusinessGroup  string `gorm:"column:business_group" json:"businessGroup"`    // 所属的业务分组国标编号
	CreateTime     string `gorm:"column:create_time" json:"createTime"`          // 创建时间
	UpdateTime     string `gorm:"column:update_time" json:"updateTime"`          // 更新时间
	CivilCode      string `gorm:"column:civil_code" json:"civilCode"`            // 行政区划
}

// TableName 指定数据库表名
func (g *Group) TableName() string {
	return "group_gb28181pro"
}

// NewGroupFromChannel 从 DeviceChannel 创建 Group 实例
func NewGroupFromChannel(channel *DeviceChannel) *Group {
	gbCode := DecodeGBCode(channel.DeviceID)
	if gbCode == nil || (gbCode.TypeCode != "215" && gbCode.TypeCode != "216") {
		return nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	group := &Group{
		Name:       channel.Name,
		DeviceID:   channel.DeviceID,
		CreateTime: now,
		UpdateTime: now,
	}

	switch gbCode.TypeCode {
	case "215":
		group.BusinessGroup = channel.DeviceID
	case "216":
		group.BusinessGroup = channel.BusinessGroupID // 注意：需要在 DeviceChannel 中添加 BusinessGroupID 字段
		group.ParentDeviceID = channel.ParentID
	}

	if group.BusinessGroup == "" {
		return nil
	}

	return group
}

// CompareTo 实现比较功能
func (g *Group) CompareTo(other *Group) int {
	thisID, _ := strconv.Atoi(g.DeviceID)
	otherID, _ := strconv.Atoi(other.DeviceID)
	return thisID - otherID
}
