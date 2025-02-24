// Package gb28181 实现了GB28181协议相关的功能
package gb28181

import (
	"time"
)

// PlatformModel 表示GB28181平台的配置信息。
// 包含了平台的基本信息、SIP服务配置、设备信息、认证信息等。
// 用于存储和管理GB28181平台的所有相关参数。
type PlatformModel struct {
	ID                      uint32 `gorm:"primaryKey;autoIncrement"`                                        // ID表示数据库中的唯一标识符
	Enable                  bool   `gorm:"column:enable" json:"enable"`                                     // Enable表示该平台配置是否启用
	Name                    string `gorm:"column:name" json:"name"`                                         // Name表示平台的名称
	ServerGBID              string `gorm:"column:server_gb_id" json:"serverGBId"`                           // ServerGBID表示SIP服务器的国标编码
	ServerGBDomain          string `gorm:"column:server_gb_domain" json:"serverGBDomain"`                   // ServerGBDomain表示SIP服务器的国标域
	ServerIP                string `gorm:"column:server_ip" json:"serverIp"`                                // ServerIP表示SIP服务器的IP地址
	ServerPort              int    `gorm:"column:server_port" json:"serverPort"`                            // ServerPort表示SIP服务器的端口号
	DeviceGBID              string `gorm:"column:device_gb_id" json:"deviceGBId"`                           // DeviceGBID表示设备的国标编号
	DeviceIP                string `gorm:"column:device_ip" json:"deviceIp"`                                // DeviceIP表示设备的IP地址
	DevicePort              int    `gorm:"column:device_port" json:"devicePort"`                            // DevicePort表示设备的端口号
	Username                string `gorm:"column:username" json:"username"`                                 // Username表示SIP认证的用户名，默认使用设备国标编号
	Password                string `gorm:"column:password" json:"password"`                                 // Password表示SIP认证的密码
	Expires                 int    `gorm:"column:expires" json:"expires"`                                   // Expires表示注册的过期时间，单位为秒
	KeepTimeout             int    `gorm:"column:keep_timeout" json:"keepTimeout"`                          // KeepTimeout表示心跳超时时间，单位为秒
	Transport               string `gorm:"column:transport" json:"transport"`                               // Transport表示传输协议类型
	CharacterSet            string `gorm:"column:character_set" json:"characterSet"`                        // CharacterSet表示字符集编码
	PTZ                     bool   `gorm:"column:ptz" json:"ptz"`                                           // PTZ表示是否允许云台控制
	RTCP                    bool   `gorm:"column:rtcp" json:"rtcp"`                                         // RTCP表示是否启用RTCP流保活
	Status                  bool   `gorm:"column:status" json:"status"`                                     // Status表示平台当前的在线状态
	ChannelCount            int    `gorm:"column:channel_count" json:"channelCount"`                        // ChannelCount表示通道数量
	CatalogSubscribe        bool   `gorm:"column:catalog_subscribe" json:"catalogSubscribe"`                // CatalogSubscribe表示是否已订阅目录信息
	AlarmSubscribe          bool   `gorm:"column:alarm_subscribe" json:"alarmSubscribe"`                    // AlarmSubscribe表示是否已订阅报警信息
	MobilePositionSubscribe bool   `gorm:"column:mobile_position_subscribe" json:"mobilePositionSubscribe"` // MobilePositionSubscribe表示是否已订阅移动位置信息
	CatalogGroup            int    `gorm:"column:catalog_group" json:"catalogGroup"`                        // CatalogGroup表示目录分组大小，每次向上级发送通道数量
	UpdateTime              string `gorm:"column:update_time" json:"updateTime"`                            // UpdateTime表示最后更新时间
	CreateTime              string `gorm:"column:create_time" json:"createTime"`                            // CreateTime表示创建时间
	AsMessageChannel        bool   `gorm:"column:as_message_channel" json:"asMessageChannel"`               // AsMessageChannel表示是否作为消息通道使用
	SendStreamIP            string `gorm:"column:send_stream_ip" json:"sendStreamIp"`                       // SendStreamIP表示点播回复200OK时使用的IP地址
	AutoPushChannel         *bool  `gorm:"column:auto_push_channel" json:"autoPushChannel"`                 // AutoPushChannel表示是否自动推送通道变化
	CatalogWithPlatform     int    `gorm:"column:catalog_with_platform" json:"catalogWithPlatform"`         // CatalogWithPlatform表示目录信息是否包含平台信息(0:关闭,1:打开)
	CatalogWithGroup        int    `gorm:"column:catalog_with_group" json:"catalogWithGroup"`               // CatalogWithGroup表示目录信息是否包含分组信息(0:关闭,1:打开)
	CatalogWithRegion       int    `gorm:"column:catalog_with_region" json:"catalogWithRegion"`             // CatalogWithRegion表示目录信息是否包含行政区划(0:关闭,1:打开)
	CivilCode               string `gorm:"column:civil_code" json:"civilCode"`                              // CivilCode表示行政区划代码
	Manufacturer            string `gorm:"column:manufacturer" json:"manufacturer"`                         // Manufacturer表示平台厂商
	Model                   string `gorm:"column:model" json:"model"`                                       // Model表示平台型号
	Address                 string `gorm:"column:address" json:"address"`                                   // Address表示平台安装地址
	RegisterWay             int    `gorm:"column:register_way" json:"registerWay"`                          // RegisterWay表示注册方式(1:标准认证注册,2:口令认证,3:数字证书双向认证,4:数字证书单向认证)
	Secrecy                 int    `gorm:"column:secrecy" json:"secrecy"`                                   // Secrecy表示保密属性(0:不涉密,1:涉密)
}

// TableName 指定数据库表名
func (p *PlatformModel) TableName() string {
	return "platform_gb28181pro"
}

// NewPlatform 创建并返回一个新的Platform实例。
// 该函数会初始化Platform结构体，并设置一些默认值：
// - RegisterWay默认设置为1（标准认证注册模式）
// - Secrecy默认设置为0（不涉密）
// 返回值为指向新创建的Platform实例的指针。
func NewPlatform() *PlatformModel {
	now := time.Now().Format("2006-01-02 15:04:05")
	return &PlatformModel{
		RegisterWay: 1, // 默认使用标准认证注册模式
		Secrecy:     0, // 默认为不涉密
		CreateTime:  now,
		UpdateTime:  now,
	}
}
