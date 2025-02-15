package gb28181

// CivilCode 行政区划编码信息
type CivilCode struct {
	Code       string `json:"code"`       // 行政区划编码
	Name       string `json:"name"`       // 行政区划名称
	ParentCode string `json:"parentCode"` // 父级行政区划编码
}

// NewCivilCodeFromArray 从字符串数组创建 CivilCode 实例
func NewCivilCodeFromArray(infoArray []string) *CivilCode {
	if len(infoArray) < 2 {
		return nil
	}

	civilCode := &CivilCode{
		Code: infoArray[0],
		Name: infoArray[1],
	}

	// 如果有父级编码
	if len(infoArray) > 2 && infoArray[2] != "" {
		civilCode.ParentCode = infoArray[2]
	}

	return civilCode
}
