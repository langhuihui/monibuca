package util

import (
	"fmt"
	"testing"
)

func TestConvertRuneToEn(t *testing.T) {
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

	// 测试 SplitRuneString
	t.Log("测试 SplitRuneString 函数:")
	for _, str := range testCases {
		segments := SplitRuneString(str)
		t.Logf("原始字符串: %q\n", str)
		for _, seg := range segments {
			t.Logf("  片段: %q, 类型: %d\n", seg.Text, seg.StrType)
		}
		fmt.Println()
	}

	// 测试 ConvertRuneToEn
	fmt.Println("\n测试 ConvertRuneToEn 函数:")
	for _, str := range testCases {
		converted := ConvertRuneToEn(str)
		t.Logf("原始字符串: %q, 转换后: %q\n", str, converted)
	}
}
