package gb28181

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	// CatalogXML 获取设备列表xml样式
	CatalogXML = `<?xml version="1.0"?><Query>
<CmdType>Catalog</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
</Query>
`
	// RecordInfoXML 获取录像文件列表xml样式
	RecordInfoXML = `<?xml version="1.0"?><Query>
<CmdType>RecordInfo</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
<StartTime>%s</StartTime>
<EndTime>%s</EndTime>
<Secrecy>0</Secrecy>
<Type>all</Type>
</Query>
`
	// DeviceInfoXML 查询设备详情xml样式
	DeviceInfoXML = `<?xml version="1.0"?><Query>
<CmdType>DeviceInfo</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
</Query>
`
	// DevicePositionXML 订阅设备位置
	DevicePositionXML = `<?xml version="1.0"?><Query>
<CmdType>MobilePosition</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
<Interval>%d</Interval>
</Query>
`
	AlarmResponseXML = `<?xml version="1.0"?><Response>
<CmdType>Alarm</CmdType>
<SN>17430</SN>
<DeviceID>%s</DeviceID>
</Response>
`
	KeepAliveXML = `<?xml version="1.0"?>
<Notify>
<CmdType>Keepalive</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
<Status>OK</Status>
</Notify>
`
	ChannelOnStatus  ChannelStatus = "ON"
	ChannelOffStatus ChannelStatus = "OFF"
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
func BuildDeviceInfoXML(sn int, id string) []byte {
	return toGB2312(fmt.Sprintf(DeviceInfoXML, sn, id))
}

// BuildCatalogXML 获取NVR下设备列表指令
func BuildCatalogXML(sn int, id string) []byte {
	return toGB2312(fmt.Sprintf(CatalogXML, sn, id))
}

// BuildRecordInfoXML 获取录像文件列表指令
func BuildRecordInfoXML(sn int, id string, start, end int64) []byte {
	return toGB2312(fmt.Sprintf(RecordInfoXML, sn, id, intTotime(start).Format("2006-01-02T15:04:05"), intTotime(end).Format("2006-01-02T15:04:05")))
}

// BuildDevicePositionXML 订阅设备位置
func BuildDevicePositionXML(sn int, id string, interval int) []byte {
	return toGB2312(fmt.Sprintf(DevicePositionXML, sn, id, interval))
}

func BuildAlarmResponseXML(id string) []byte {
	return toGB2312(fmt.Sprintf(AlarmResponseXML, id))
}

func BuildKeepAliveXML(sn int, id string) []byte {
	return toGB2312(fmt.Sprintf(KeepAliveXML, sn, id))
}

type (
	ChannelStatus string
	Record        struct {
		DeviceID  string
		Name      string
		FilePath  string
		Address   string
		StartTime string
		EndTime   string
		Secrecy   int
		Type      string
	}
	ChannelInfo struct {
		DeviceID     string // 通道ID
		ParentID     string
		Name         string
		Manufacturer string
		Model        string
		Owner        string
		CivilCode    string
		Address      string
		Port         int
		Parental     int
		SafetyWay    int
		RegisterWay  int
		Secrecy      int
		Status       ChannelStatus
	}
	Message struct {
		XMLName      xml.Name
		CmdType      string
		SN           int // 请求序列号，一般用于对应 request 和 response
		DeviceID     string
		DeviceName   string
		Manufacturer string
		Model        string
		Channel      string
		DeviceList   []ChannelInfo `xml:"DeviceList>Item"`
		RecordList   []Record      `xml:"RecordList>Item"`
		SumNum       int           // 录像结果的总数 SumNum，录像结果会按照多条消息返回，可用于判断是否全部返回
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
