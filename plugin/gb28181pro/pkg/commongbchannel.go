package gb28181

import (
	"fmt"
)

// CommonGBChannel 通用国标通道信息
type CommonGBChannel struct {
	// 数据库自增ID
	GbID int `json:"gbId" gorm:"column:gb_id"`

	// 国标编码
	GbDeviceID string `json:"gbDeviceId" gorm:"column:gb_device_id"`

	// 国标名称
	GbName string `json:"gbName" gorm:"column:gb_name"`

	// 国标设备厂商
	GbManufacturer string `json:"gbManufacturer" gorm:"column:gb_manufacturer"`

	// 国标设备型号
	GbModel string `json:"gbModel" gorm:"column:gb_model"`

	// 国标设备归属
	GbOwner string `json:"gbOwner" gorm:"column:gb_owner"`

	// 国标行政区域
	GbCivilCode string `json:"gbCivilCode" gorm:"column:gb_civil_code"`

	// 国标警区
	GbBlock string `json:"gbBlock" gorm:"column:gb_block"`

	// 国标安装地址
	GbAddress string `json:"gbAddress" gorm:"column:gb_address"`

	// 国标是否有子设备
	GbParental int `json:"gbParental" gorm:"column:gb_parental"`

	// 国标父节点ID
	GbParentID string `json:"gbParentId" gorm:"column:gb_parent_id"`

	// 国标信令安全模式
	GbSafetyWay int `json:"gbSafetyWay" gorm:"column:gb_safety_way"`

	// 国标注册方式
	GbRegisterWay int `json:"gbRegisterWay" gorm:"column:gb_register_way"`

	// 国标证书序列号
	GbCertNum string `json:"gbCertNum" gorm:"column:gb_cert_num"`

	// 国标证书有效标识
	GbCertifiable int `json:"gbCertifiable" gorm:"column:gb_certifiable"`

	// 国标无效原因码
	GbErrCode int `json:"gbErrCode" gorm:"column:gb_err_code"`

	// 国标证书终止有效期
	GbEndTime string `json:"gbEndTime" gorm:"column:gb_end_time"`

	// 国标保密属性
	GbSecrecy int `json:"gbSecrecy" gorm:"column:gb_secrecy"`

	// 国标IP地址
	GbIPAddress string `json:"gbIpAddress" gorm:"column:gb_ip_address"`

	// 国标端口
	GbPort int `json:"gbPort" gorm:"column:gb_port"`

	// 国标密码
	GbPassword string `json:"gbPassword" gorm:"column:gb_password"`

	// 国标状态
	GbStatus string `json:"gbStatus" gorm:"column:gb_status"`

	// 国标经度
	GbLongitude float64 `json:"gbLongitude" gorm:"column:gb_longitude"`

	// 国标纬度
	GbLatitude float64 `json:"gbLatitude" gorm:"column:gb_latitude"`

	// 国标业务分组ID
	GbBusinessGroupID string `json:"gbBusinessGroupId" gorm:"column:gb_business_group_id"`

	// 国标云台类型
	GbPTZType int `json:"gbPtzType" gorm:"column:gb_ptz_type"`

	// 国标位置类型
	GbPositionType int `json:"gbPositionType" gorm:"column:gb_position_type"`

	// 国标房间类型
	GbRoomType int `json:"gbRoomType" gorm:"column:gb_room_type"`

	// 国标用途类型
	GbUseType int `json:"gbUseType" gorm:"column:gb_use_type"`

	// 国标补光类型
	GbSupplyLightType int `json:"gbSupplyLightType" gorm:"column:gb_supply_light_type"`

	// 国标方向类型
	GbDirectionType int `json:"gbDirectionType" gorm:"column:gb_direction_type"`

	// 国标分辨率
	GbResolution string `json:"gbResolution" gorm:"column:gb_resolution"`

	// 国标下载速度
	GbDownloadSpeed string `json:"gbDownloadSpeed" gorm:"column:gb_download_speed"`

	// 国标空域编码能力
	GbSvcSpaceSupportMod int `json:"gbSvcSpaceSupportMod" gorm:"column:gb_svc_space_support_mod"`

	// 国标时域编码能力
	GbSvcTimeSupportMode int `json:"gbSvcTimeSupportMode" gorm:"column:gb_svc_time_support_mode"`

	// 关联的国标设备数据库ID
	GbDeviceDbID int `json:"gbDeviceDbId" gorm:"column:gb_device_db_id"`

	// 二进制保存的录制计划
	RecordPlan int64 `json:"recordPlan" gorm:"column:record_plan"`

	// 关联的推流ID
	StreamPushID int `json:"streamPushId" gorm:"column:stream_push_id"`

	// 关联的拉流代理ID
	StreamProxyID int `json:"streamProxyId" gorm:"column:stream_proxy_id"`

	// 创建时间
	CreateTime string `json:"createTime" gorm:"column:create_time"`

	// 更新时间
	UpdateTime string `json:"updateTime" gorm:"column:update_time"`

	// 流ID，存在表示正在推流
	StreamID string `json:"streamId" xml:"-"`

	// 是否含有音频
	HasAudio bool `json:"hasAudio" xml:"-"`
}

// Build 构建通道信息
func (c *CommonGBChannel) Build(deviceID string, name string, manufacturer string, model string, owner string,
	civilCode string, block string, address string, parentID string) {
	// TODO: 实现构建逻辑
}

// GetFullContent 获取完整的通道信息内容
func (c *CommonGBChannel) GetFullContent(deviceID string, name string, parentID string, event string) string {
	content := "<Item>\n"
	content += fmt.Sprintf("<DeviceID>%s</DeviceID>\n", deviceID)
	content += fmt.Sprintf("<Name>%s</Name>\n", name)

	if len(deviceID) > 8 {
		deviceType := deviceID[10:13]
		switch deviceType {
		case "200":
			// 业务分组目录项
			if c.GbManufacturer != "" {
				content += fmt.Sprintf("<Manufacturer>%s</Manufacturer>\n", c.GbManufacturer)
			}
			if c.GbModel != "" {
				content += fmt.Sprintf("<Model>%s</Model>\n", c.GbModel)
			}
			if c.GbOwner != "" {
				content += fmt.Sprintf("<Owner>%s</Owner>\n", c.GbOwner)
			}
			if c.GbCivilCode != "" {
				content += fmt.Sprintf("<CivilCode>%s</CivilCode>\n", c.GbCivilCode)
			}
			if c.GbAddress != "" {
				content += fmt.Sprintf("<Address>%s</Address>\n", c.GbAddress)
			}
			if c.GbRegisterWay != 0 {
				content += fmt.Sprintf("<RegisterWay>%d</RegisterWay>\n", c.GbRegisterWay)
			}
			content += fmt.Sprintf("<Secrecy>%d</Secrecy>\n", c.GbSecrecy)

		case "215":
			// 业务分组
			if c.GbCivilCode != "" {
				content += fmt.Sprintf("<CivilCode>%s</CivilCode>\n", c.GbCivilCode)
			}
			content += fmt.Sprintf("<ParentID>%s</ParentID>\n", parentID)

		case "216":
			// 虚拟组织目录项
			if c.GbCivilCode != "" {
				content += fmt.Sprintf("<CivilCode>%s</CivilCode>\n", c.GbCivilCode)
			}
			if c.GbParentID != "" {
				content += fmt.Sprintf("<ParentID>%s</ParentID>\n", c.GbParentID)
			}
			content += fmt.Sprintf("<BusinessGroupID>%s</BusinessGroupID>\n", c.GbBusinessGroupID)

		default:
			// 其他类型
			if c.GbManufacturer != "" {
				content += fmt.Sprintf("<Manufacturer>%s</Manufacturer>\n", c.GbManufacturer)
			}
			if c.GbModel != "" {
				content += fmt.Sprintf("<Model>%s</Model>\n", c.GbModel)
			}
			if c.GbOwner != "" {
				content += fmt.Sprintf("<Owner>%s</Owner>\n", c.GbOwner)
			}
			if c.GbCivilCode != "" {
				content += fmt.Sprintf("<CivilCode>%s</CivilCode>\n", c.GbCivilCode)
			}
			if c.GbAddress != "" {
				content += fmt.Sprintf("<Address>%s</Address>\n", c.GbAddress)
			}
			if c.GbParentID != "" {
				content += fmt.Sprintf("<ParentID>%s</ParentID>\n", c.GbParentID)
			}
			content += fmt.Sprintf("<Parental>%d</Parental>\n", c.GbParental)
			if c.GbSafetyWay != 0 {
				content += fmt.Sprintf("<SafetyWay>%d</SafetyWay>\n", c.GbSafetyWay)
			}
			if c.GbRegisterWay != 0 {
				content += fmt.Sprintf("<RegisterWay>%d</RegisterWay>\n", c.GbRegisterWay)
			}
			if c.GbCertNum != "" {
				content += fmt.Sprintf("<CertNum>%s</CertNum>\n", c.GbCertNum)
			}
			if c.GbCertifiable != 0 {
				content += fmt.Sprintf("<Certifiable>%d</Certifiable>\n", c.GbCertifiable)
			}
			if c.GbErrCode != 0 {
				content += fmt.Sprintf("<ErrCode>%d</ErrCode>\n", c.GbErrCode)
			}
			if c.GbEndTime != "" {
				content += fmt.Sprintf("<EndTime>%s</EndTime>\n", c.GbEndTime)
			}
			content += fmt.Sprintf("<Secrecy>%d</Secrecy>\n", c.GbSecrecy)
			if c.GbIPAddress != "" {
				content += fmt.Sprintf("<IPAddress>%s</IPAddress>\n", c.GbIPAddress)
			}
			if c.GbPort != 0 {
				content += fmt.Sprintf("<Port>%d</Port>\n", c.GbPort)
			}
			if c.GbPassword != "" {
				content += fmt.Sprintf("<Password>%s</Password>\n", c.GbPassword)
			}
			if c.GbStatus != "" {
				content += fmt.Sprintf("<Status>%s</Status>\n", c.GbStatus)
			}
			if c.GbLongitude != 0 {
				content += fmt.Sprintf("<Longitude>%f</Longitude>\n", c.GbLongitude)
			}
			if c.GbLatitude != 0 {
				content += fmt.Sprintf("<Latitude>%f</Latitude>\n", c.GbLatitude)
			}

			// Info 部分
			content += "<Info>\n"
			if c.GbPTZType != 0 {
				content += fmt.Sprintf("  <PTZType>%d</PTZType>\n", c.GbPTZType)
			}
			if c.GbPositionType != 0 {
				content += fmt.Sprintf("  <PositionType>%d</PositionType>\n", c.GbPositionType)
			}
			if c.GbRoomType != 0 {
				content += fmt.Sprintf("  <RoomType>%d</RoomType>\n", c.GbRoomType)
			}
			if c.GbUseType != 0 {
				content += fmt.Sprintf("  <UseType>%d</UseType>\n", c.GbUseType)
			}
			if c.GbSupplyLightType != 0 {
				content += fmt.Sprintf("  <SupplyLightType>%d</SupplyLightType>\n", c.GbSupplyLightType)
			}
			if c.GbDirectionType != 0 {
				content += fmt.Sprintf("  <DirectionType>%d</DirectionType>\n", c.GbDirectionType)
			}
			if c.GbResolution != "" {
				content += fmt.Sprintf("  <Resolution>%s</Resolution>\n", c.GbResolution)
			}
			if c.GbBusinessGroupID != "" {
				content += fmt.Sprintf("  <BusinessGroupID>%s</BusinessGroupID>\n", c.GbBusinessGroupID)
			}
			if c.GbDownloadSpeed != "" {
				content += fmt.Sprintf("  <DownloadSpeed>%s</DownloadSpeed>\n", c.GbDownloadSpeed)
			}
			if c.GbSvcSpaceSupportMod != 0 {
				content += fmt.Sprintf("  <SVCSpaceSupportMode>%d</SVCSpaceSupportMode>\n", c.GbSvcSpaceSupportMod)
			}
			if c.GbSvcTimeSupportMode != 0 {
				content += fmt.Sprintf("  <SVCTimeSupportMode>%d</SVCTimeSupportMode>\n", c.GbSvcTimeSupportMode)
			}
			content += "</Info>\n"
		}
	}

	if event != "" {
		content += fmt.Sprintf("<Event>%s</Event>\n", event)
	}
	content += "</Item>\n"
	return content
}

// Encode 编码通道信息
func (c *CommonGBChannel) Encode(deviceID string, event string) string {
	if event == "" {
		return c.GetFullContent(deviceID, c.GbName, "", "")
	}

	switch event {
	case "DEL", "DEFECT", "VLOST", "ON", "OFF":
		return fmt.Sprintf("<Item>\n<DeviceID>%s</DeviceID>\n<Event>%s</Event>\n</Item>\n", deviceID, event)
	case "ADD", "UPDATE":
		return c.GetFullContent(deviceID, c.GbName, "", event)
	default:
		return ""
	}
}
