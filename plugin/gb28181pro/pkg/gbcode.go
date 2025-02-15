package gb28181

// GbCode 国标编码对象
type GbCode struct {
	CenterCode   string `json:"centerCode"`   // 中心编码,由监控中心所在地的行政区划代码确定,符合GB/T2260—2007的要求
	IndustryCode string `json:"industryCode"` // 行业编码
	TypeCode     string `json:"typeCode"`     // 类型编码
	NetCode      string `json:"netCode"`      // 网络标识
	SN           string `json:"sn"`           // 序号
}

// DecodeGBCode 解析国标编号
func DecodeGBCode(code string) *GbCode {
	if code == "" || len(code) != 20 {
		return nil
	}

	return &GbCode{
		CenterCode:   code[0:8],
		IndustryCode: code[8:10],
		TypeCode:     code[10:13],
		NetCode:      code[13:14],
		SN:           code[14:],
	}
}

// Encode 编码为完整的国标编号
func (g *GbCode) Encode() string {
	return g.CenterCode + g.IndustryCode + g.TypeCode + g.NetCode + g.SN
}
