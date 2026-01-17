package gb28181

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	// CatalogXML 获取设备列表xml样式
	CatalogXML = `<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>Catalog</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
</Query>
`
	// SubscribeCatalogXML 获取设备列表xml样式
	SubscribeCatalogXML = `<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>Catalog</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
</Query>
`
	// RecordInfoXML 获取录像文件列表xml样式
	RecordInfoXML = `<?xml version="1.0"?>
<Query>
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
	DeviceInfoXML = `<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>DeviceInfo</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
</Query>
`
	ConfigDownloadXML = `<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>ConfigDownload</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
<ConfigType>BasicParam/VideoParamOpt/SVACEncodeConfig/SVACDecodeConfig</ConfigType>
</Query>
`
	// DeviceStatusXML 查询设备详情xml样式
	DeviceStatusXML = `<?xml version="1.0" encoding="%s"?>
<Query>
<CmdType>DeviceStatus</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
</Query>
`
	// DevicePositionXML 订阅设备位置
	DevicePositionXML = `<?xml version="1.0"?>
<Query>
<CmdType>MobilePosition</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
<Interval>%d</Interval>
</Query>
`
	// PresetQueryXML 查询预置位指令
	PresetQueryXML = `<?xml version="1.0"?>
<Query>
<CmdType>PresetQuery</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
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

// BuildDeviceInfoXML 获取设备详情指令
func BuildConfigDownloadXML(sn int, id string, charset string) []byte {
	return toGB2312(fmt.Sprintf(ConfigDownloadXML, charset, sn, id))
}

// BuildDeviceStatusXML 获取设备详情指令
func BuildDeviceStatusXML(sn int, id string, charset string) []byte {
	return toGB2312(fmt.Sprintf(DeviceStatusXML, charset, sn, id))
}

// BuildCatalogXML 获取NVR下设备列表指令
func BuildSubscribeCatalogXML(charset string, sn int, id string) []byte {
	return toGB2312(fmt.Sprintf(SubscribeCatalogXML, charset, sn, id))
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
		XMLName      xml.Name
		CmdType      string
		SN           int // 请求序列号，一般用于对应 request 和 response
		DeviceID     string
		StartTime    string `xml:"StartTime"`    // 查询开始时间
		EndTime      string `xml:"EndTime"`      // 查询结束时间
		Secrecy      int    `xml:"Secrecy"`      // 保密属性
		Type         string `xml:"Type"`         // 录像类型
		Longitude    string // 经度
		Latitude     string // 纬度
		DeviceName   string
		Manufacturer string
		Model        string
		Channel      string
		Firmware     string
		DeviceList   struct {
			DeviceChannelList []DeviceChannel `xml:"Item"`
			DeviceNum         int             `xml:"Num,attr"` // 将 Num 属性映射到 DeviceNum
		} `xml:"DeviceList"`
		RecordList struct {
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
		BasicParam    struct {
			Expiration         int    `xml:"Expiration"`         //注册过期时间
			HeartBeatInterval  int    `xml:"HeartBeatInterval"`  // 心跳间隔
			HeartBeatCount     int    `xml:"HeartBeatCount"`     // 心跳次数
			PositionCapability int    `xml:"PositionCapability"` //定位功能支持情况  取值:0-不支持;1-支持GPS定位;2-支持北斗定位(可选,默认取值为0)
			Name               string `xml:"Name"`
		} `xml:"BasicParam"`
		Info struct {
			AlarmType string `xml:"AlarmType"` // 报警类型
		} `xml:"Info"`
		Record string `xml:"Record"` //录像状态,DeviceStatus响应可选,On,Off
		Online string `xml:"Online"` //是否在线,DeviceStatus响应必选
		Status string `xml:"Status"` //是否正常工作,DeviceStatus响应必选
		Reason string `xml:"Reason"` //不正常工作原因，DeviceStatus响应可选
		Encode string `xml:"Encode"` //是否编码，DeviceStatus响应可选，On,Off
	}

	PresetItem struct {
		PresetID   string `xml:"PresetID"`
		PresetName string `xml:"PresetName"`
	}

	MobilePositionNotify struct {
		XMLName   xml.Name `xml:"Notify"`
		CmdType   string   `xml:"CmdType"`
		SN        int      `xml:"SN"`
		DeviceID  string   `xml:"DeviceID"`
		Time      string   `xml:"Time"`
		Longitude float64  `xml:"Longitude"`
		Latitude  float64  `xml:"Latitude"`
	}

	AlarmNotify struct {
		XMLName       xml.Name `xml:"Notify"`
		CmdType       string   `xml:"CmdType"`
		SN            int      `xml:"SN"`
		DeviceID      string   `xml:"DeviceID"`
		AlarmPriority string   `xml:"AlarmPriority"`
		AlarmTime     string   `xml:"AlarmTime"`
		AlarmMethod   string   `xml:"AlarmMethod"`
		Info          struct {
			AlarmType string `xml:"AlarmType"`
		} `xml:"Info"`
	}
)

// DecodeXML 根据指定的字符集解码XML
// charset: 字符集，如 "GB2312", "UTF-8" 等，如果为空则默认使用GB2312
func DecodeXML(v any, body []byte, charset string) error {
	// 标准化字符集名称
	if charset == "" {
		charset = "GB2312" // 默认使用GB2312
	}
	charset = strings.ToUpper(strings.TrimSpace(charset))

	// 提取XML声明中的encoding
	declaredEncoding := extractXMLEncoding(body)
	
	// 判断是否需要转换编码
	needConvert := false
	if declaredEncoding != "" {
		declaredEncoding = strings.ToUpper(strings.TrimSpace(declaredEncoding))
		// 如果声明的encoding与配置的charset不一致，需要转换
		if !isSameCharset(declaredEncoding, charset) {
			needConvert = true
		}
	}
	
	var finalBody []byte
	if needConvert {
		// 需要转换：根据配置的charset决定如何处理
		switch charset {
		case "GB2312", "GBK", "GB18030":
			// 配置说实际是GB2312，先用GBK解码转成UTF-8
			reader := transform.NewReader(bytes.NewReader(body), simplifiedchinese.GBK.NewDecoder())
			utf8Body, err := io.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("convert from %s to UTF-8 failed: %v", charset, err)
			}
			// 修改XML声明为UTF-8
			finalBody = replaceXMLEncoding(utf8Body, "UTF-8")
		case "UTF-8", "UTF8":
			// 配置说实际是UTF-8，但声明不是UTF-8
			// 只需要修改XML声明，不转换内容
			finalBody = replaceXMLEncoding(body, "UTF-8")
		default:
			return fmt.Errorf("unsupported charset: %s", charset)
		}
	} else {
		// 不需要转换，直接使用原始body
		finalBody = body
	}
	
	// 解析XML
	decoder := xml.NewDecoder(bytes.NewReader(finalBody))
	decoder.CharsetReader = func(declaredCharset string, input io.Reader) (io.Reader, error) {
		// 如果XML声明的是GB2312/GBK，提供GBK解码器
		dc := strings.ToUpper(strings.TrimSpace(declaredCharset))
		switch dc {
		case "GB2312", "GBK", "GB18030":
			return transform.NewReader(input, simplifiedchinese.GBK.NewDecoder()), nil
		default:
			return input, nil
		}
	}
	
	err := decoder.Decode(v)
	if err != nil {
		return fmt.Errorf("decode XML failed: %v", err)
	}

	return nil
}

// extractXMLEncoding 从XML中提取encoding声明
func extractXMLEncoding(body []byte) string {
	// 查找 encoding="xxx" 或 encoding='xxx'
	bodyStr := string(body[:min(len(body), 200)]) // 只检查前200字节
	if idx := strings.Index(bodyStr, "encoding="); idx >= 0 {
		start := idx + 9
		if start < len(bodyStr) {
			quote := bodyStr[start]
			if quote == '"' || quote == '\'' {
				end := strings.IndexByte(bodyStr[start+1:], quote)
				if end >= 0 {
					return bodyStr[start+1 : start+1+end]
				}
			}
		}
	}
	return ""
}

// isSameCharset 判断两个字符集是否相同
func isSameCharset(a, b string) bool {
	a = strings.ToUpper(strings.TrimSpace(a))
	b = strings.ToUpper(strings.TrimSpace(b))
	
	// GB2312, GBK, GB18030 视为相同
	if (a == "GB2312" || a == "GBK" || a == "GB18030") &&
		(b == "GB2312" || b == "GBK" || b == "GB18030") {
		return true
	}
	
	// UTF-8 和 UTF8 视为相同
	if (a == "UTF-8" || a == "UTF8") && (b == "UTF-8" || b == "UTF8") {
		return true
	}
	
	return a == b
}

// replaceXMLEncoding 替换XML声明中的encoding
func replaceXMLEncoding(body []byte, newEncoding string) []byte {
	bodyStr := string(body)
	if idx := strings.Index(bodyStr, "encoding="); idx >= 0 {
		start := idx + 9
		if start < len(bodyStr) {
			quote := bodyStr[start]
			if quote == '"' || quote == '\'' {
				end := strings.IndexByte(bodyStr[start+1:], quote)
				if end >= 0 {
					// 替换encoding值
					return []byte(bodyStr[:start+1] + newEncoding + bodyStr[start+1+end:])
				}
			}
		}
	}
	return body
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
