package util

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

// StringSegment 表示字符串片段及其类型
type StringSegment struct {
	Text    string
	StrType int // 0: 英文/数字, 1: 中文, 2: 其他语言
}

// splitChineseString 将字符串切割成不同类型的片段
func splitChineseString(s string) []StringSegment {
	var segments []StringSegment
	var builder strings.Builder
	var currentType int // 当前正在构建的片段类型

	// 获取字符类型
	getCharType := func(r rune) int {
		switch {
		case unicode.Is(unicode.Han, r):
			return 1 // 中文
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			return 2 // 日语假名
		case unicode.Is(unicode.Hangul, r):
			return 2 // 韩语
		case unicode.Is(unicode.Thai, r):
			return 2 // 泰语
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			return 0 // 英文或数字
		default:
			// 其他 Unicode 字符，如果不是空格或标点，也认为是其他语言
			if !unicode.IsSpace(r) && !unicode.IsPunct(r) {
				return 2
			}
			// 对于空格和标点，跟随当前类型
			return currentType
		}
	}

	// 添加当前片段到结果中
	addSegment := func() {
		if builder.Len() > 0 {
			segments = append(segments, StringSegment{
				Text:    builder.String(),
				StrType: currentType,
			})
			builder.Reset()
		}
	}

	runes := []rune(s)
	if len(runes) == 0 {
		return segments
	}

	// 初始化第一个字符
	firstChar := runes[0]
	currentType = getCharType(firstChar)
	builder.WriteRune(firstChar)

	// 处理剩余字符
	for i := 1; i < len(runes); i++ {
		currentChar := runes[i]
		charType := getCharType(currentChar)

		// 如果字符类型发生变化，添加新片段
		if charType != currentType {
			addSegment()
			currentType = charType
		}

		builder.WriteRune(currentChar)
	}

	// 添加最后一个片段
	addSegment()

	return segments
}

// ConvertChineseToPinyin 将字符串中的中文转换为拼音，其他语言转换为 base64
func ConvertChineseToPinyin(s string) string {
	segments := splitChineseString(s)
	var result strings.Builder
	a := pinyin.NewArgs()

	for i, seg := range segments {
		switch seg.StrType {
		case 0: // 英文/数字
			result.WriteString(seg.Text)
		case 1: // 中文
			pinyinSlice := pinyin.LazyPinyin(seg.Text, a)
			result.WriteString(strings.Join(pinyinSlice, ""))
		case 2: // 其他语言，使用 base64 编码（不带填充）
			result.WriteString(base64.RawURLEncoding.EncodeToString([]byte(seg.Text)))
		}

		// 如果不是最后一个片段，且下一个片段类型不同，添加空格
		if i < len(segments)-1 && segments[i+1].StrType != seg.StrType {
			result.WriteString("")
		}
	}
	return result.String()
}

func main() {
	// 测试用例
	testCases := []string{
		"aa哈哈哈",
		"hello世界",
		"123你好",
		"混合字符串abc中文123",
		"纯中文测试",
		"onlyEnglish",

		"Hello世界こんにちは", // 添加包含其他语言的测试用例
	}

	// 测试 splitChineseString
	fmt.Println("测试 splitChineseString 函数:")
	for _, str := range testCases {
		segments := splitChineseString(str)
		fmt.Printf("原始字符串: %q\n", str)
		for _, seg := range segments {
			fmt.Printf("  片段: %q, 类型: %d\n", seg.Text, seg.StrType)
		}
		fmt.Println()
	}

	// 测试 ConvertChineseToPinyin
	fmt.Println("\n测试 ConvertChineseToPinyin 函数:")
	for _, str := range testCases {
		converted := ConvertChineseToPinyin(str)
		fmt.Printf("原始字符串: %q, 转换后: %q\n", str, converted)
	}
}
