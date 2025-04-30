package gb28181

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	// CatalogXML 获取设备列表xml样式
	CatalogXML = `<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>Catalog</CmdType>
<SN>%d</SN>
<DeviceId>%s</DeviceId>
</Query>
`
	// RecordInfoXML 获取录像文件列表xml样式
	RecordInfoXML = `<?xml version="1.0"?>
<Query>
<CmdType>RecordInfo</CmdType>
<SN>%d</SN>
<DeviceId>%s</DeviceId>
<StartTime>%s</StartTime>
<EndTime>%s</EndTime>
<Secrecy>0</Secrecy>
<Type>all</Type>
</Query>
`
	// DeviceInfoXML 查询设备详情xml样式
	DeviceInfoXML = `<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>DeviceInfo</CmdType>
<SN>%d</SN>
<DeviceId>%s</DeviceId>
</Query>
`
	// DeviceStatusXML 查询设备详情xml样式
	DeviceStatusXML = `<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>DeviceStatus</CmdType>
<SN>%d</SN>
<DeviceId>%s</DeviceId>
</Query>
`
	// DevicePositionXML 订阅设备位置
	DevicePositionXML = `<?xml version="1.0"?>
<Query>
<CmdType>MobilePosition</CmdType>
<SN>%d</SN>
<DeviceId>%s</DeviceId>
<Interval>%d</Interval>
</Query>
`
	// PresetQueryXML 查询预置位指令
	PresetQueryXML = `<?xml version="1.0"?>
<Query>
<CmdType>PresetQuery</CmdType>
<SN>%d</SN>
<DeviceId>%s</DeviceId>
</Query>
`
	AlarmResponseXML = `<?xml version="1.0"?><Response>
<CmdType>Alarm</CmdType>
<SN>17430</SN>
<DeviceId>%s</DeviceId>
</Response>
`
	KeepAliveXML = `<?xml version="1.0"?>
<Notify>
<CmdType>Keepalive</CmdType>
<SN>%d</SN>
<DeviceId>%s</DeviceId>
<Status>OK</Status>
</Notify>
`
)

func intTotime(t int64) time.Time {
	tstr := strconv.FormatInt(t, 10)
	if len(tstr) == 10 {
		return time.Unix(t, 0)
	}
	if len(tstr) == 13 {
		return time.UnixMilli(t)
	}
	return time.Now()
}

func toGB2312(s string) []byte {
	reader := transform.NewReader(strings.NewReader(s), simplifiedchinese.GBK.NewEncoder())
	d, _ := io.ReadAll(reader)
	return d
}

// BuildDeviceInfoXML 获取设备详情指令
func BuildDeviceInfoXML(sn int, id string, charset string) []byte {
	return toGB2312(fmt.Sprintf(DeviceInfoXML, charset, sn, id))
}

// BuildDeviceStatusXML 获取设备详情指令
func BuildDeviceStatusXML(sn int, id string, charset string) []byte {
	return toGB2312(fmt.Sprintf(DeviceStatusXML, charset, sn, id))
}

// BuildCatalogXML 获取NVR下设备列表指令
func BuildCatalogXML(charset string, sn int, id string) []byte {
	return toGB2312(fmt.Sprintf(CatalogXML, charset, sn, id))
}

// BuildRecordInfoXML 获取录像文件列表指令
func BuildRecordInfoXML(sn int, id string, start, end int64) []byte {
	return toGB2312(fmt.Sprintf(RecordInfoXML, sn, id, intTotime(start).Format("2006-01-02T15:04:05"), intTotime(end).Format("2006-01-02T15:04:05")))
}

// BuildDevicePositionXML 订阅设备位置
func BuildDevicePositionXML(sn int, id string, interval int) []byte {
	return toGB2312(fmt.Sprintf(DevicePositionXML, sn, id, interval))
}

// BuildPresetQueryXML 构建预置位查询XML
func BuildPresetQueryXML(sn int, id string) []byte {
	return toGB2312(fmt.Sprintf(PresetQueryXML, sn, id))
}

func BuildAlarmResponseXML(id string) []byte {
	return toGB2312(fmt.Sprintf(AlarmResponseXML, id))
}

func BuildKeepAliveXML(sn int, id string) []byte {
	return toGB2312(fmt.Sprintf(KeepAliveXML, sn, id))
}

type (
	Message struct {
		XMLName           xml.Name
		CmdType           string
		SN                int // 请求序列号，一般用于对应 request 和 response
		DeviceID          string
		DeviceName        string
		Manufacturer      string
		Model             string
		Channel           string
		Firmware          string
		DeviceChannelList []DeviceChannel `xml:"DeviceList>Item"`
		RecordList        struct {
			Num  int          `xml:"Num,attr"`
			Item []RecordItem `xml:"Item"`
		} `xml:"RecordList"`
		PresetList struct {
			Num  int          `xml:"Num,attr"`
			Item []PresetItem `xml:"Item"`
		} `xml:"PresetList"`
		SumNum   int       // 录像结果的总数 SumNum，录像结果会按照多条消息返回，可用于判断是否全部返回
		Name     string    // 设备/通道名称
		LastTime time.Time `xml:"LastTime"` // 最后时间
		// 报警相关字段
		AlarmPriority string `xml:"AlarmPriority"` // 报警级别
		AlarmMethod   string `xml:"AlarmMethod"`   // 报警方式
		AlarmTime     string `xml:"AlarmTime"`     // 报警时间
		Info          struct {
			AlarmType string `xml:"AlarmType"` // 报警类型
		} `xml:"Info"`
	}

	PresetItem struct {
		PresetID   string `xml:"PresetID"`
		PresetName string `xml:"PresetName"`
	}
)

func DecodeXML(v any, body []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.CharsetReader = charset.NewReaderLabel
	err := decoder.Decode(v)
	if err != nil {
		decoder = xml.NewDecoder(transform.NewReader(bytes.NewReader(body), simplifiedchinese.GBK.NewDecoder()))
		decoder.CharsetReader = charset.NewReaderLabel
		return decoder.Decode(v)
	}
	return nil
}
