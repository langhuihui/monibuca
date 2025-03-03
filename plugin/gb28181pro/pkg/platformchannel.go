package gb28181

// PlatformChannel 表示平台通道信息
type PlatformChannel struct {
	CommonGBChannel `gorm:"-"` // 通过组合继承 CommonGBChannel 的字段

	Id                             int     // 数据库自增长ID
	PlatformId                     int     // 平台ID
	DeviceChannelId                int     // 设备通道ID
	CustomDeviceId                 string  `gorm:"default:null"` // 国标-编码
	CustomName                     string  `gorm:"default:null"` // 国标-名称
	CustomManufacturer             string  `gorm:"default:null"` // 国标-设备厂商
	CustomModel                    string  `gorm:"default:null"` // 国标-设备型号
	CustomOwner                    string  `gorm:"default:null"` // 国标-设备归属
	CustomCivilCode                string  // 国标-行政区域
	CustomBlock                    string  // 国标-警区
	CustomAddress                  string  `gorm:"default:null"` // 国标-安装地址
	CustomParental                 int     // 国标-是否有子设备
	CustomParentId                 string  // 国标-父节点ID
	CustomSafetyWay                int     // 国标-信令安全模式
	CustomRegisterWay              int     // 国标-注册方式
	CustomCertNum                  int     // 国标-证书序列号
	CustomCertifiable              int     // 国标-证书有效标识
	CustomErrCode                  int     // 国标-无效原因码
	CustomEndTime                  int     // 国标-证书终止有效期
	CustomSecurityLevelCode        string  // 国标-摄像机安全能力等级代码
	CustomSecrecy                  int     // 国标-保密属性
	CustomIpAddress                string  // 国标-设备/系统IPv4/IPv6地址
	CustomPort                     int     // 国标-设备/系统端口
	CustomPassword                 string  // 国标-设备口令
	CustomStatus                   string  // 国标-设备状态
	CustomLongitude                float64 // 国标-经度
	CustomLatitude                 float64 // 国标-纬度
	CustomBusinessGroupId          string  // 国标-虚拟组织所属的业务分组ID
	CustomPtzType                  int     // 国标-摄像机结构类型
	CustomPositionType             int     // 国标-摄像机位置类型扩展
	CustomPhotoelectricImagingType string  // 国标-摄像机光电成像类型
	CustomCapturePositionType      string  // 国标-摄像机采集部位类型
	CustomRoomType                 int     // 国标-摄像机安装位置室外、室内属性
	CustomUseType                  int     // 国标-用途属性
	CustomSupplyLightType          int     // 国标-摄像机补光属性
	CustomDirectionType            int     // 国标-摄像机监视方位属性
	CustomResolution               string  // 国标-摄像机支持的分辨率
	CustomStreamNumberList         string  // 国标-摄像机支持的码流编号列表
	CustomDownloadSpeed            string  // 国标-下载倍速
	CustomSvcSpaceSupportMod       int     // 国标-空域编码能力
	CustomSvcTimeSupportMode       int     // 国标-时域编码能力
	CustomSsvcRatioSupportList     string  // 国标-SSVC增强层与基本层比例能力
	CustomMobileDeviceType         int     // 国标-移动采集设备类型
	CustomHorizontalFieldAngle     float64 // 国标-摄像机水平视场角
	CustomVerticalFieldAngle       float64 // 国标-摄像机竖直视场角
	CustomMaxViewDistance          float64 // 国标-摄像机可视距离
	CustomGrassrootsCode           string  // 国标-基层组织编码
	CustomPoType                   int     // 国标-监控点位类型
	CustomPoCommonName             string  // 国标-点位俗称
	CustomMac                      string  // 国标-设备MAC地址
	CustomFunctionType             string  // 国标-摄像机卡口功能类型
	CustomEncodeType               string  // 国标-摄像机视频编码格式
	CustomInstallTime              string  // 国标-摄像机安装使用时间
	CustomManagementUnit           string  // 国标-摄像机所属管理单位名称
	CustomContactInfo              string  // 国标-摄像机所属管理单位联系方式
	CustomRecordSaveDays           int     // 国标-录像保存天数
	CustomIndustrialClassification string  // 国标-国民经济行业分类代码
}

// BuildFromCommonGBChannel 从 CommonGBChannel 构建 PlatformChannel 实例
func BuildFromCommonGBChannel(common *CommonGBChannel, platformId int) *PlatformChannel {
	if common == nil {
		return nil
	}

	return &PlatformChannel{
		CommonGBChannel:    *common,
		PlatformId:         platformId,
		CustomDeviceId:     common.GbDeviceID,
		CustomName:         common.GbName,
		CustomManufacturer: common.GbManufacturer,
		CustomModel:        common.GbModel,
		CustomOwner:        common.GbOwner,
		CustomCivilCode:    common.GbCivilCode,
		CustomAddress:      common.GbAddress,
		CustomParental:     common.GbParental,
		CustomParentId:     common.GbParentID,
		CustomSafetyWay:    common.GbSafetyWay,
		CustomRegisterWay:  common.GbRegisterWay,
		CustomSecrecy:      common.GbSecrecy,
		CustomStatus:       common.GbStatus,
	}
}

// ToCommonGBChannel 将 PlatformChannel 转换为 CommonGBChannel
func (p *PlatformChannel) ToCommonGBChannel() *CommonGBChannel {
	return &CommonGBChannel{
		GbDeviceID:     p.CustomDeviceId,
		GbName:         p.CustomName,
		GbManufacturer: p.CustomManufacturer,
		GbModel:        p.CustomModel,
		GbOwner:        p.CustomOwner,
		GbCivilCode:    p.CustomCivilCode,
		GbAddress:      p.CustomAddress,
		GbParental:     p.CustomParental,
		GbParentID:     p.CustomParentId,
		GbSafetyWay:    p.CustomSafetyWay,
		GbRegisterWay:  p.CustomRegisterWay,
		GbSecrecy:      p.CustomSecrecy,
		GbStatus:       p.CustomStatus,
	}
}

// TableName 返回数据库表名
func (*PlatformChannel) TableName() string {
	return "platform_channel_gb28181pro"
}
