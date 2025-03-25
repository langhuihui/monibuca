package gb28181

import (
	"strings"
	"sync"
)

// CivilCodeUtil 行政区划编码工具类
type CivilCodeUtil struct {
	// 用于消息的缓存
	civilCodeMap sync.Map // map[string]*CivilCode
}

var (
	instance *CivilCodeUtil
	once     sync.Once
)

// GetInstance 获取单例实例
func GetInstance() *CivilCodeUtil {
	once.Do(func() {
		instance = &CivilCodeUtil{}
	})
	return instance
}

// Add 添加多个行政区划编码
func (c *CivilCodeUtil) Add(civilCodeList []*CivilCode) {
	if len(civilCodeList) > 0 {
		for _, civilCode := range civilCodeList {
			c.civilCodeMap.Store(civilCode.Code, civilCode)
		}
	}
}

// AddOne 添加单个行政区划编码
func (c *CivilCodeUtil) AddOne(civilCode *CivilCode) {
	c.civilCodeMap.Store(civilCode.Code, civilCode)
}

// GetParentCode 获取父级编码
func (c *CivilCodeUtil) GetParentCode(code string) *CivilCode {
	if len(code) > 8 {
		return nil
	}
	if len(code) == 8 {
		parentCode := code[:6]
		if value, ok := c.civilCodeMap.Load(parentCode); ok {
			return value.(*CivilCode)
		}
		return nil
	}

	if value, ok := c.civilCodeMap.Load(code); ok {
		civilCode := value.(*CivilCode)
		if civilCode.ParentCode == "" {
			return nil
		}
		if parentValue, ok := c.civilCodeMap.Load(civilCode.ParentCode); ok {
			return parentValue.(*CivilCode)
		}
	}
	return nil
}

// GetCivilCode 获取行政区划编码对象
func (c *CivilCodeUtil) GetCivilCode(code string) *CivilCode {
	if len(code) > 8 {
		return nil
	}
	if value, ok := c.civilCodeMap.Load(code); ok {
		return value.(*CivilCode)
	}
	return nil
}

// GetAllParentCode 获取所有父级编码
func (c *CivilCodeUtil) GetAllParentCode(civilCode string) []*CivilCode {
	var civilCodeList []*CivilCode
	parentCode := c.GetParentCode(civilCode)
	if parentCode != nil {
		civilCodeList = append(civilCodeList, parentCode)
		allParentCode := c.GetAllParentCode(parentCode.Code)
		if len(allParentCode) > 0 {
			civilCodeList = append(civilCodeList, allParentCode...)
		}
	}
	return civilCodeList
}

// IsEmpty 判断是否为空
func (c *CivilCodeUtil) IsEmpty() bool {
	empty := true
	c.civilCodeMap.Range(func(key, value interface{}) bool {
		empty = false
		return false
	})
	return empty
}

// Size 获取大小
func (c *CivilCodeUtil) Size() int {
	size := 0
	c.civilCodeMap.Range(func(key, value interface{}) bool {
		size++
		return true
	})
	return size
}

// GetAllChild 获取所有子节点
func (c *CivilCodeUtil) GetAllChild(parent string) []*Region {
	var result []*Region
	c.civilCodeMap.Range(func(key, value interface{}) bool {
		civilCode := value.(*CivilCode)
		if parent == "" {
			if strings.TrimSpace(civilCode.ParentCode) == "" {
				result = append(result, NewRegion(civilCode.Code, civilCode.Name, civilCode.ParentCode))
			}
		} else if civilCode.ParentCode == parent {
			result = append(result, NewRegion(civilCode.Code, civilCode.Name, civilCode.ParentCode))
		}
		return true
	})
	return result
}
