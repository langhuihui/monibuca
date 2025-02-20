package gb28181

import "time"

// DeviceAlarm 报警信息结构体
type DeviceAlarm struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id" description:"数据库id"`
	DeviceID         string    `json:"deviceId" description:"设备的国标编号"`
	DeviceName       string    `json:"deviceName" description:"设备名称"`
	ChannelID        string    `json:"channelId" description:"通道的国标编号"`
	AlarmPriority    string    `json:"alarmPriority" description:"报警级别, 1为一级警情, 2为二级警情, 3为三级警情, 4为四级警情"`
	AlarmMethod      string    `json:"alarmMethod" description:"报警方式,1为电话报警,2为设备报警,3为短信报警,4为GPS报警,5为视频报警,6为设备故障报警,7其他报警"`
	AlarmTime        time.Time `json:"alarmTime" description:"报警时间"`
	AlarmDescription string    `json:"alarmDescription" description:"报警内容描述"`
	Longitude        float64   `json:"longitude" description:"经度"`
	Latitude         float64   `json:"latitude" description:"纬度"`
	AlarmType        string    `json:"alarmType" description:"报警类型"`
	CreateTime       time.Time `json:"createTime" description:"创建时间"`
}

// GetAlarmPriorityDescription 获取报警级别描述
func (a *DeviceAlarm) GetAlarmPriorityDescription() string {
	switch a.AlarmPriority {
	case "1":
		return "一级警情"
	case "2":
		return "二级警情"
	case "3":
		return "三级警情"
	case "4":
		return "四级警情"
	default:
		return a.AlarmPriority
	}
}

// GetAlarmMethodDescription 获取报警方式描述
func (a *DeviceAlarm) GetAlarmMethodDescription() string {
	var desc []rune
	for _, c := range a.AlarmMethod {
		switch c {
		case '1':
			desc = append(desc, []rune("-电话报警")...)
		case '2':
			desc = append(desc, []rune("-设备报警")...)
		case '3':
			desc = append(desc, []rune("-短信报警")...)
		case '4':
			desc = append(desc, []rune("-GPS报警")...)
		case '5':
			desc = append(desc, []rune("-视频报警")...)
		case '6':
			desc = append(desc, []rune("-设备故障报警")...)
		case '7':
			desc = append(desc, []rune("-其他报警")...)
		}
	}
	if len(desc) > 0 {
		return string(desc[1:]) // 去掉第一个'-'
	}
	return ""
}

// GetAlarmTypeDescription 获取报警类型描述
func (a *DeviceAlarm) GetAlarmTypeDescription() string {
	if a.AlarmType == "" {
		return ""
	}

	// 检查报警方式
	methodMap := make(map[string]bool)
	for _, c := range a.AlarmMethod {
		methodMap[string(c)] = true
	}

	// 根据不同的报警方式返回对应的描述
	if methodMap["2"] { // 设备报警
		switch a.AlarmType {
		case "1":
			return "视频丢失报警"
		case "2":
			return "设备防拆报警"
		case "3":
			return "存储设备磁盘满报警"
		case "4":
			return "设备高温报警"
		case "5":
			return "设备低温报警"
		}
	}

	if methodMap["5"] || methodMap["6"] { // 视频报警或设备故障报警
		switch a.AlarmType {
		case "1":
			return "人工视频报警"
		case "2":
			return "运动目标检测报警"
		case "3":
			return "遗留物检测报警"
		case "4":
			return "物体移除检测报警"
		case "5":
			return "绊线检测报警"
		case "6":
			return "入侵检测报警"
		case "7":
			return "逆行检测报警"
		case "8":
			return "徘徊检测报警"
		case "9":
			return "流量统计报警"
		case "10":
			return "密度检测报警"
		case "11":
			return "视频异常检测报警"
		case "12":
			return "快速移动报警"
		}
	}

	return a.AlarmType
}

// TableName 返回数据库表名
func (DeviceAlarm) TableName() string {
	return "devicealarm_gb28181pro"
}
