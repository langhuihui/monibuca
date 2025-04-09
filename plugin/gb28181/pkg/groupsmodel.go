package gb28181

import (
	"time"
)

// GroupsModel 表示分组结构
type GroupsModel struct {
	ID         int       `gorm:"primaryKey;autoIncrement" json:"id"`   // ID表示数据库中的唯一标识符
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"` // CreateTime表示记录创建时间
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"` // UpdateTime表示记录更新时间
	Name       string    `gorm:"column:name" json:"name"`              // Name表示分组名称
	PID        int       `gorm:"column:pid;default:0" json:"pid"`      // PID表示父分组ID
	Level      int       `gorm:"column:level;default:0" json:"level"`  // Level表示分组层级
}

// TableName 指定数据库表名
func (g *GroupsModel) TableName() string {
	return "groups"
}

// NewGroup 创建并返回一个新的GroupsModel实例
func NewGroup(name string, pid int, level int) *GroupsModel {
	now := time.Now()
	return &GroupsModel{
		Name:       name,
		PID:        pid,
		Level:      level,
		CreateTime: now,
		UpdateTime: now,
	}
}

// NewRootGroup 创建根分组实例
func NewRootGroup() *GroupsModel {
	return NewGroup("根", 0, 0)
}

// InitRootGroup 初始化根分组记录
// 如果数据库中不存在根分组，则创建一个
func InitRootGroup(db interface{}) error {
	// db 参数应为 *gorm.DB 类型
	// 使用类型断言获取 DB 实例
	// 这里使用 interface{} 是为了避免直接依赖 GORM
	type DBAPI interface {
		First(dest interface{}, conds ...interface{}) error
		Create(value interface{}) error
	}

	if gdb, ok := db.(DBAPI); ok {
		root := &GroupsModel{}

		// 检查是否存在根分组
		err := gdb.First(root, "pid = ? AND level = ?", 0, 0)
		if err != nil {
			// 如果不存在，则创建一个根分组
			rootGroup := NewRootGroup()
			return gdb.Create(rootGroup)
		}

		return nil
	}

	return nil
}

// AutoMigrateAll 执行分组及分组-通道关联的自动迁移，并初始化根组织
// 此函数应在插件初始化时调用，一次完成所有相关表的迁移
func AutoMigrateAll(db interface{}) error {
	// db 参数应为 *gorm.DB 类型
	// 使用类型断言获取 DB 实例
	type DBAPI interface {
		AutoMigrate(dst ...interface{}) error
		First(dest interface{}, conds ...interface{}) error
		Create(value interface{}) error
	}

	if gdb, ok := db.(DBAPI); ok {
		// 执行表结构自动迁移 - 分组表和分组-通道关联表
		if err := gdb.AutoMigrate(&GroupsModel{}, &GroupsChannelModel{}); err != nil {
			return err
		}

		// 检查是否存在根分组
		root := &GroupsModel{}
		err := gdb.First(root, "pid = ? AND level = ?", 0, 0)
		if err != nil {
			// 如果不存在，则创建一个根分组
			rootGroup := NewRootGroup()
			return gdb.Create(rootGroup)
		}

		return nil
	}

	return nil
}

// BeforeCreate GORM钩子，在创建记录前设置创建时间和更新时间
func (g *GroupsModel) BeforeCreate() error {
	now := time.Now()
	g.CreateTime = now
	g.UpdateTime = now
	return nil
}

// BeforeUpdate GORM钩子，在更新记录前设置更新时间
func (g *GroupsModel) BeforeUpdate() error {
	g.UpdateTime = time.Now()
	return nil
}
