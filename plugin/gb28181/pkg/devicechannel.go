package gb28181

import (
	"strconv"
	"time"
)

// ChannelStatus 通道状态类型
type ChannelStatus string

const (
	ChannelOnStatus  ChannelStatus = "ON"
	ChannelOffStatus ChannelStatus = "OFF"
)

// DeviceChannel 设备通道信息
type DeviceChannel struct {
	//CommonGBChannel // 通过组合继承 CommonGBChannel 的字段

	ID                 string        `gorm:"primaryKey" json:"Id"` // 数据库自增长ID
	ChannelId          string        `json:"channelId" xml:"ChannelID"`
	CustomChannelId    string        `json:"customChannelId" xml:"CustomChannelID"`             // 自定义通道ID
	DeviceId           string        `json:"deviceId" xml:"DeviceID"`                           // 设备国标编号
	ParentId           string        `json:"parentId" xml:"ParentID"`                           // 父节点ID
	Name               string        `json:"name" xml:"Name"`                                   // 通道名称
	CustomName         string        `json:"customName" xml:"CustomName"`                       // 自定义名称
	Manufacturer       string        `json:"manufacturer" xml:"Manufacturer"`                   // 设备厂商
	Model              string        `json:"model" xml:"Model"`                                 // 设备型号
	Owner              string        `json:"owner" xml:"Owner"`                                 // 设备归属
	CivilCode          string        `json:"civilCode" xml:"CivilCode"`                         // 行政区域
	Block              string        `json:"block" xml:"Block"`                                 // 警区
	Address            string        `json:"address" xml:"Address"`                             // 安装地址
	Port               int           `json:"port" xml:"Port"`                                   // 端口
	Parental           int           `json:"parental" xml:"Parental"`                           // 是否有子设备
	SafetyWay          int           `json:"safetyWay" xml:"SafetyWay"`                         // 信令安全模式
	RegisterWay        int           `json:"registerWay" xml:"RegisterWay"`                     // 注册方式
	CertNum            string        `json:"certNum" xml:"CertNum"`                             // 证书序列号
	Certifiable        int           `json:"certifiable" xml:"Certifiable"`                     // 证书有效标识
	ErrCode            int           `json:"errCode" xml:"ErrCode"`                             // 无效原因码
	EndTime            string        `json:"endTime" xml:"EndTime"`                             // 证书终止有效期
	Secrecy            int           `json:"secrecy" xml:"Secrecy"`                             // 保密属性
	IPAddress          string        `json:"ipAddress" xml:"IPAddress"`                         // 设备/系统IP地址
	Password           string        `json:"password" xml:"Password"`                           // 设备口令
	PTZType            int           `json:"ptzType" xml:"Info>PTZType"`                        // 摄像机类型
	PositionType       int           `json:"positionType" xml:"Info>PositionType"`              // 摄像机位置类型
	RoomType           int           `json:"roomType" xml:"Info>RoomType"`                      // 安装位置室内外属性
	UseType            int           `json:"useType" xml:"Info>UseType"`                        // 用途属性
	SupplyLightType    int           `json:"supplyLightType" xml:"Info>SupplyLightType"`        // 摄像机补光属性
	DirectionType      int           `json:"directionType" xml:"Info>DirectionType"`            // 摄像机监视方位属性
	Resolution         string        `json:"resolution" xml:"Info>Resolution"`                  // 摄像机支持的分辨率
	BusinessGroupID    string        `json:"businessGroupId" xml:"Info>BusinessGroupID"`        // 虚拟组织所属的业务分组ID
	DownloadSpeed      string        `json:"downloadSpeed" xml:"Info>DownloadSpeed"`            // 下载倍速
	SVCSpaceSupportMod int           `json:"svcSpaceSupportMod" xml:"Info>SVCSpaceSupportMode"` // 空域编码能力
	SVCTimeSupportMode int           `json:"svcTimeSupportMode" xml:"Info>SVCTimeSupportMode"`  // 时域编码能力
	StreamPushID       int           `json:"streamPushId"`                                      // 关联的推流ID
	StreamProxyID      int           `json:"streamProxyId"`                                     // 关联的拉流代理ID
	CreateTime         string        `json:"createTime"`                                        // 创建时间
	Status             ChannelStatus `json:"status" xml:"Status"`                               // 设备状态
	Longitude          float64
	Latitude           float64

	PTZTypeText string  `json:"ptzTypeText"` // 云台类型描述字符串
	GbLongitude float64 `json:"gbLongitude"`
	GbLatitude  float64 `json:"gbLatitude"`
	StreamPath  string  `json:"streamPath"` // 拉流代理的流路径
}

// SetPTZType 设置云台类型并更新描述文本
func (d *DeviceChannel) SetPTZType(ptzType int) {
	d.PTZType = ptzType
	switch ptzType {
	case 0:
		d.PTZTypeText = "未知"
	case 1:
		d.PTZTypeText = "球机"
	case 2:
		d.PTZTypeText = "半球"
	case 3:
		d.PTZTypeText = "固定枪机"
	case 4:
		d.PTZTypeText = "遥控枪机"
	case 5:
		d.PTZTypeText = "遥控半球"
	case 6:
		d.PTZTypeText = "多目设备的全景/拼接通道"
	case 7:
		d.PTZTypeText = "多目设备的分割通道"
	}
}

// DecodeWithOnlyDeviceID 仅解码设备ID
func DecodeWithOnlyDeviceID(element interface{}) (*DeviceChannel, error) {
	// TODO: 实现仅解码设备ID的逻辑
	return nil, nil
}

// TableName 指定数据库表名
func (d *DeviceChannel) TableName() string {
	return "gb28181_channel"
}

// NewDeviceChannel 创建新的设备通道实例
func NewDeviceChannel() *DeviceChannel {
	now := time.Now().Format("2006-01-02 15:04:05")
	return &DeviceChannel{
		CreateTime: now,
		Status:     ChannelOffStatus,
	}
}

// Encode 生成通道信息的XML内容
func (d *DeviceChannel) Encode(event string, serverDeviceID string) string {
	if event == "" {
		return d.getFullContent("", serverDeviceID)
	}

	switch event {
	case "DEL", "DEFECT", "VLOST":
		return "<Item>\n" +
			"<DeviceID>" + d.DeviceId + "</DeviceID>\n" +
			"<Event>" + event + "</Event>\n" +
			"</Item>\n"
	case "ON", "OFF":
		return "<Item>\n" +
			"<DeviceID>" + d.DeviceId + "</DeviceID>\n" +
			"<Event>" + event + "</Event>\n" +
			"</Item>\n"
	case "ADD", "UPDATE":
		return d.getFullContent(event, serverDeviceID)
	default:
		return ""
	}
}

// getFullContent 生成完整的通道信息XML内容
func (d *DeviceChannel) getFullContent(event string, serverDeviceID string) string {
	content := "<Item>\n" +
		"<DeviceID>" + d.DeviceId + "</DeviceID>\n" +
		"<Name>" + d.Name + "</Name>\n"

	if len(d.DeviceId) > 8 {
		typeCode := d.DeviceId[10:13]
		switch typeCode {
		case "200":
			// 业务分组目录项
			if d.Manufacturer != "" {
				content += "<Manufacturer>" + d.Manufacturer + "</Manufacturer>\n"
			}
			if d.Model != "" {
				content += "<Model>" + d.Model + "</Model>\n"
			}
			if d.Owner != "" {
				content += "<Owner>" + d.Owner + "</Owner>\n"
			}
			if d.CivilCode != "" {
				content += "<CivilCode>" + d.CivilCode + "</CivilCode>\n"
			}
			if d.Address != "" {
				content += "<Address>" + d.Address + "</Address>\n"
			}
			if d.RegisterWay != 0 {
				content += "<RegisterWay>" + strconv.Itoa(d.RegisterWay) + "</RegisterWay>\n"
			}
			if d.Secrecy != 0 {
				content += "<Secrecy>" + strconv.Itoa(d.Secrecy) + "</Secrecy>\n"
			}
		case "215":
			// 业务分组
			if d.CivilCode != "" {
				content += "<CivilCode>" + d.CivilCode + "</CivilCode>\n"
			}
			content += "<ParentID>" + serverDeviceID + "</ParentID>\n"
		case "216":
			// 虚拟组织目录项
			if d.CivilCode != "" {
				content += "<CivilCode>" + d.CivilCode + "</CivilCode>\n"
			}
			if d.ParentId != "" {
				content += "<ParentID>" + d.ParentId + "</ParentID>\n"
			}
			content += "<BusinessGroupID>" + d.BusinessGroupID + "</BusinessGroupID>\n"
		default:
			// 其他类型
			d.appendCommonInfo(&content)
		}
	}

	if event != "" {
		content += "<Event>" + event + "</Event>\n"
	}
	content += "</Item>\n"
	return content
}

// appendCommonInfo 添加通用信息到XML内容
func (d *DeviceChannel) appendCommonInfo(content *string) {
	if d.Manufacturer != "" {
		*content += "<Manufacturer>" + d.Manufacturer + "</Manufacturer>\n"
	}
	if d.Model != "" {
		*content += "<Model>" + d.Model + "</Model>\n"
	}
	if d.Owner != "" {
		*content += "<Owner>" + d.Owner + "</Owner>\n"
	}
	if d.CivilCode != "" {
		*content += "<CivilCode>" + d.CivilCode + "</CivilCode>\n"
	}
	if d.Block != "" {
		*content += "<Block>" + d.Block + "</Block>\n"
	}
	if d.Address != "" {
		*content += "<Address>" + d.Address + "</Address>\n"
	}
	if d.ParentId != "" {
		*content += "<ParentID>" + d.ParentId + "</ParentID>\n"
	}
	if d.SafetyWay != 0 {
		*content += "<SafetyWay>" + strconv.Itoa(d.SafetyWay) + "</SafetyWay>\n"
	}
	if d.RegisterWay != 0 {
		*content += "<RegisterWay>" + strconv.Itoa(d.RegisterWay) + "</RegisterWay>\n"
	}
	if d.CertNum != "" {
		*content += "<CertNum>" + d.CertNum + "</CertNum>\n"
	}
	if d.Certifiable != 0 {
		*content += "<Certifiable>" + strconv.Itoa(d.Certifiable) + "</Certifiable>\n"
	}
	if d.ErrCode != 0 {
		*content += "<ErrCode>" + strconv.Itoa(d.ErrCode) + "</ErrCode>\n"
	}
	if d.EndTime != "" {
		*content += "<EndTime>" + d.EndTime + "</EndTime>\n"
	}
	if d.IPAddress != "" {
		*content += "<IPAddress>" + d.IPAddress + "</IPAddress>\n"
	}
	if d.Port != 0 {
		*content += "<Port>" + strconv.Itoa(d.Port) + "</Port>\n"
	}
	if d.Password != "" {
		*content += "<Password>" + d.Password + "</Password>\n"
	}
	if d.Status != "" {
		*content += "<Status>" + string(d.Status) + "</Status>\n"
	}
	if d.GbLongitude != 0 {
		*content += "<Longitude>" + strconv.FormatFloat(d.GbLongitude, 'f', -1, 64) + "</Longitude>\n"
	}
	if d.GbLatitude != 0 {
		*content += "<Latitude>" + strconv.FormatFloat(d.GbLatitude, 'f', -1, 64) + "</Latitude>\n"
	}

	// 添加Info标签内的信息
	*content += "<Info>\n"
	d.appendInfoContent(content)
	*content += "</Info>\n"
}

// appendInfoContent 添加Info标签内的信息到XML内容
func (d *DeviceChannel) appendInfoContent(content *string) {
	if d.PTZType != 0 {
		*content += "  <PTZType>" + strconv.Itoa(d.PTZType) + "</PTZType>\n"
	}
	if d.PositionType != 0 {
		*content += "  <PositionType>" + strconv.Itoa(d.PositionType) + "</PositionType>\n"
	}
	if d.RoomType != 0 {
		*content += "  <RoomType>" + strconv.Itoa(d.RoomType) + "</RoomType>\n"
	}
	if d.UseType != 0 {
		*content += "  <UseType>" + strconv.Itoa(d.UseType) + "</UseType>\n"
	}
	if d.SupplyLightType != 0 {
		*content += "  <SupplyLightType>" + strconv.Itoa(d.SupplyLightType) + "</SupplyLightType>\n"
	}
	if d.DirectionType != 0 {
		*content += "  <DirectionType>" + strconv.Itoa(d.DirectionType) + "</DirectionType>\n"
	}
	if d.Resolution != "" {
		*content += "  <Resolution>" + d.Resolution + "</Resolution>\n"
	}
	if d.BusinessGroupID != "" {
		*content += "  <BusinessGroupID>" + d.BusinessGroupID + "</BusinessGroupID>\n"
	}
	if d.DownloadSpeed != "" {
		*content += "  <DownloadSpeed>" + d.DownloadSpeed + "</DownloadSpeed>\n"
	}
	if d.SVCSpaceSupportMod != 0 {
		*content += "  <SVCSpaceSupportMode>" + strconv.Itoa(d.SVCSpaceSupportMod) + "</SVCSpaceSupportMode>\n"
	}
	if d.SVCTimeSupportMode != 0 {
		*content += "  <SVCTimeSupportMode>" + strconv.Itoa(d.SVCTimeSupportMode) + "</SVCTimeSupportMode>\n"
	}
}

func (d *DeviceChannel) GetKey() string {
	return d.ID
}
