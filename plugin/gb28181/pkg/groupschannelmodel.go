package gb28181

// GroupsChannelModel 表示分组与通道的关联关系
type GroupsChannelModel struct {
	ID        int    `gorm:"primaryKey;autoIncrement" json:"id"`       // ID表示数据库中的唯一标识符
	GroupID   int    `gorm:"column:group_id;index" json:"groupId"`     // GroupID表示关联的分组ID
	ChannelID string `gorm:"column:channel_id;index" json:"channelId"` // ChannelID表示关联的通道ID
	DeviceID  string `gorm:"column:device_id;index" json:"deviceId"`   // DeviceID表示关联的设备ID
}

// TableName 指定数据库表名
func (g *GroupsChannelModel) TableName() string {
	return "groups_channel"
}

// NewGroupsChannel 创建并返回一个新的GroupsChannelModel实例
func NewGroupsChannel(groupID int, channelID string, deviceID string) *GroupsChannelModel {
	return &GroupsChannelModel{
		GroupID:   groupID,
		ChannelID: channelID,
		DeviceID:  deviceID,
	}
}

// FindGroupChannels 通过分组ID查找关联的通道
func FindGroupChannels(db interface{}, groupID int) ([]*GroupsChannelModel, error) {
	// db 参数应为 *gorm.DB 类型
	type DBAPI interface {
		Find(dest interface{}, conds ...interface{}) error
	}

	if gdb, ok := db.(DBAPI); ok {
		var channels []*GroupsChannelModel
		err := gdb.Find(&channels, "group_id = ?", groupID)
		return channels, err
	}

	return nil, nil
}

// FindChannelGroups 通过通道ID查找关联的分组
func FindChannelGroups(db interface{}, channelID string) ([]*GroupsChannelModel, error) {
	// db 参数应为 *gorm.DB 类型
	type DBAPI interface {
		Find(dest interface{}, conds ...interface{}) error
	}

	if gdb, ok := db.(DBAPI); ok {
		var groups []*GroupsChannelModel
		err := gdb.Find(&groups, "channel_id = ?", channelID)
		return groups, err
	}

	return nil, nil
}

// FindDeviceGroups 通过设备ID查找关联的分组
func FindDeviceGroups(db interface{}, deviceID string) ([]*GroupsChannelModel, error) {
	// db 参数应为 *gorm.DB 类型
	type DBAPI interface {
		Find(dest interface{}, conds ...interface{}) error
	}

	if gdb, ok := db.(DBAPI); ok {
		var groups []*GroupsChannelModel
		err := gdb.Find(&groups, "device_id = ?", deviceID)
		return groups, err
	}

	return nil, nil
}

// FindGroupChannelsByDevice 通过分组ID和设备ID查找关联的通道
func FindGroupChannelsByDevice(db interface{}, groupID int, deviceID string) ([]*GroupsChannelModel, error) {
	// db 参数应为 *gorm.DB 类型
	type DBAPI interface {
		Find(dest interface{}, conds ...interface{}) error
	}

	if gdb, ok := db.(DBAPI); ok {
		var channels []*GroupsChannelModel
		err := gdb.Find(&channels, "group_id = ? AND device_id = ?", groupID, deviceID)
		return channels, err
	}

	return nil, nil
}

// AutoMigrate 执行自动迁移
// 此函数应在插件初始化时调用
func AutoMigrateGroupChannel(db interface{}) error {
	// db 参数应为 *gorm.DB 类型
	// 使用类型断言获取 DB 实例
	type DBAPI interface {
		AutoMigrate(dst ...interface{}) error
	}

	if gdb, ok := db.(DBAPI); ok {
		// 执行表结构自动迁移
		if err := gdb.AutoMigrate(&GroupsChannelModel{}); err != nil {
			return err
		}
		return nil
	}

	return nil
}
