package plugin_crontab

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateTimeSlots(t *testing.T) {
	// 测试案例：周五的凌晨和上午有开启时间段
	// 字符串中1的索引是120(0点),122(2点),123(3点),125(5点),130(10点),135(15点)
	// 000000000000000000000000 - 周日(0-23小时) - 全0
	// 000000000000000000000000 - 周一(24-47小时) - 全0
	// 000000000000000000000000 - 周二(48-71小时) - 全0
	// 000000000000000000000000 - 周三(72-95小时) - 全0
	// 000000000000000000000000 - 周四(96-119小时) - 全0
	// 101101000010000100000000 - 周五(120-143小时) - 0,2,3,5,10,15点开启
	// 000000000000000000000000 - 周六(144-167小时) - 全0
	planStr := "000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000101101000010000100000000000000000000000000000000"

	now := time.Date(2023, 5, 1, 12, 0, 0, 0, time.Local) // 周一中午

	slots, err := calculateTimeSlots(planStr, now, time.Local)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(slots), "应该有5个时间段")

	// 检查结果中的时间段（按实际解析结果排序）
	assert.Equal(t, "周五", slots[0].Weekday)
	assert.Equal(t, "10:00-11:00", slots[0].TimeRange)

	assert.Equal(t, "周五", slots[1].Weekday)
	assert.Equal(t, "15:00-16:00", slots[1].TimeRange)

	assert.Equal(t, "周五", slots[2].Weekday)
	assert.Equal(t, "00:00-01:00", slots[2].TimeRange)

	assert.Equal(t, "周五", slots[3].Weekday)
	assert.Equal(t, "02:00-04:00", slots[3].TimeRange)

	assert.Equal(t, "周五", slots[4].Weekday)
	assert.Equal(t, "05:00-06:00", slots[4].TimeRange)

	// 打印出所有时间段，便于调试
	for i, slot := range slots {
		t.Logf("时间段 %d: %s %s", i, slot.Weekday, slot.TimeRange)
	}
}

func TestGetNextTimeSlotFromNow(t *testing.T) {
	// 测试案例：周五的凌晨和上午有开启时间段
	planStr := "000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000101101000010000100000000000000000000000000000000"

	// 测试1: 当前是周一，下一个时间段应该是周五凌晨0点
	now1 := time.Date(2023, 5, 1, 12, 0, 0, 0, time.Local) // 周一中午
	nextSlot1, err := getNextTimeSlotFromNow(planStr, now1, time.Local)
	assert.NoError(t, err)
	assert.NotNil(t, nextSlot1)
	assert.Equal(t, "周五", nextSlot1.Weekday)
	assert.Equal(t, "00:00-01:00", nextSlot1.TimeRange)

	// 测试2: 当前是周五凌晨1点，下一个时间段应该是周五凌晨2点
	now2 := time.Date(2023, 5, 5, 1, 30, 0, 0, time.Local) // 周五凌晨1:30
	nextSlot2, err := getNextTimeSlotFromNow(planStr, now2, time.Local)
	assert.NoError(t, err)
	assert.NotNil(t, nextSlot2)
	assert.Equal(t, "周五", nextSlot2.Weekday)
	assert.Equal(t, "02:00-04:00", nextSlot2.TimeRange)

	// 测试3: 当前是周五凌晨3点，此时正在一个时间段内
	now3 := time.Date(2023, 5, 5, 3, 0, 0, 0, time.Local) // 周五凌晨3:00
	nextSlot3, err := getNextTimeSlotFromNow(planStr, now3, time.Local)
	assert.NoError(t, err)
	assert.NotNil(t, nextSlot3)
	assert.Equal(t, "周五", nextSlot3.Weekday)
	assert.Equal(t, "02:00-04:00", nextSlot3.TimeRange)
}

func TestParsePlanFromString(t *testing.T) {
	// 测试用户提供的案例：字符串的第36-41位表示周一的时间段
	// 这个案例中，对应周一的12点、14-15点、17点和22点开启
	planStr := "000000000000000000000000000000000000101101000010000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	now := time.Now()
	slots, err := calculateTimeSlots(planStr, now, time.Local)
	assert.NoError(t, err)

	// 验证解析结果
	var foundMondaySlots bool
	for _, slot := range slots {
		if slot.Weekday == "周一" {
			foundMondaySlots = true
			t.Logf("找到周一时间段: %s", slot.TimeRange)
		}
	}
	assert.True(t, foundMondaySlots, "应该找到周一的时间段")

	// 预期的周一时间段
	var mondaySlots []string
	for _, slot := range slots {
		if slot.Weekday == "周一" {
			mondaySlots = append(mondaySlots, slot.TimeRange)
		}
	}

	// 检查是否包含预期的时间段
	expectedSlots := []string{
		"12:00-13:00",
		"14:00-16:00",
		"17:00-18:00",
		"22:00-23:00",
	}

	for _, expected := range expectedSlots {
		found := false
		for _, actual := range mondaySlots {
			if expected == actual {
				found = true
				break
			}
		}
		assert.True(t, found, "应该找到周一时间段："+expected)
	}

	// 获取下一个时间段
	nextSlot, err := getNextTimeSlotFromNow(planStr, now, time.Local)
	assert.NoError(t, err)
	if nextSlot != nil {
		t.Logf("下一个时间段: %s %s", nextSlot.Weekday, nextSlot.TimeRange)
	} else {
		t.Log("没有找到下一个时间段")
	}
}

// 手动计算字符串长度的辅助函数
func TestCountStringLength(t *testing.T) {
	str1 := "000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000101101000010000100000000000000000000000000000000"
	assert.Equal(t, 168, len(str1), "第一个测试字符串长度应为168")

	str2 := "000000000000000000000000000000000000101101000010000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	assert.Equal(t, 168, len(str2), "第二个测试字符串长度应为168")
}

// 测试用户提供的具体字符串
func TestUserProvidedPlanString(t *testing.T) {
	// 用户提供的测试字符串
	planStr := "000000000000000000000000000000000000101101000010000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	// 验证字符串长度
	assert.Equal(t, 168, len(planStr), "字符串长度应为168")

	// 解析时间段
	now := time.Now()
	slots, err := calculateTimeSlots(planStr, now, time.Local)
	assert.NoError(t, err)

	// 打印所有时间段
	t.Log("所有时间段:")
	for i, slot := range slots {
		t.Logf("%d: %s %s", i, slot.Weekday, slot.TimeRange)
	}

	// 获取下一个时间段
	nextSlot, err := getNextTimeSlotFromNow(planStr, now, time.Local)
	assert.NoError(t, err)

	if nextSlot != nil {
		t.Logf("下一个执行时间段: %s %s", nextSlot.Weekday, nextSlot.TimeRange)
		t.Logf("开始时间: %s", nextSlot.Start.AsTime().In(time.Local).Format("2006-01-02 15:04:05"))
		t.Logf("结束时间: %s", nextSlot.End.AsTime().In(time.Local).Format("2006-01-02 15:04:05"))
	} else {
		t.Log("没有找到下一个时间段")
	}

	// 验证周一的时间段
	var mondaySlots []string
	for _, slot := range slots {
		if slot.Weekday == "周一" {
			mondaySlots = append(mondaySlots, slot.TimeRange)
		}
	}

	// 预期周一应该有这些时间段
	expectedMondaySlots := []string{
		"12:00-13:00",
		"14:00-16:00",
		"17:00-18:00",
		"22:00-23:00",
	}

	assert.Equal(t, len(expectedMondaySlots), len(mondaySlots), "周一时间段数量不匹配")

	for i, expected := range expectedMondaySlots {
		if i < len(mondaySlots) {
			t.Logf("期望周一时间段 %s, 实际是 %s", expected, mondaySlots[i])
		}
	}
}

// 测试用户提供的第二个字符串
func TestUserProvidedPlanString2(t *testing.T) {
	// 用户提供的第二个测试字符串
	planStr := "000000000000000000000000000000000000000000000000000000000000001011010100001000000000000000000000000100000000000000000000000010000000000000000000000001000000000000000000"

	// 验证字符串长度
	assert.Equal(t, 168, len(planStr), "字符串长度应为168")

	// 解析时间段
	now := time.Now()
	slots, err := calculateTimeSlots(planStr, now, time.Local)
	assert.NoError(t, err)

	// 打印所有时间段并按周几分组
	weekdaySlots := make(map[string][]string)
	for _, slot := range slots {
		weekdaySlots[slot.Weekday] = append(weekdaySlots[slot.Weekday], slot.TimeRange)
	}

	t.Log("所有时间段（按周几分组）:")
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	for _, weekday := range weekdays {
		if timeRanges, ok := weekdaySlots[weekday]; ok {
			t.Logf("%s: %v", weekday, timeRanges)
		}
	}

	// 打印所有时间段的详细信息
	t.Log("\n所有时间段详细信息:")
	for i, slot := range slots {
		t.Logf("%d: %s %s", i, slot.Weekday, slot.TimeRange)
	}

	// 获取下一个时间段
	nextSlot, err := getNextTimeSlotFromNow(planStr, now, time.Local)
	assert.NoError(t, err)

	if nextSlot != nil {
		t.Logf("\n下一个执行时间段: %s %s", nextSlot.Weekday, nextSlot.TimeRange)
		t.Logf("开始时间: %s", nextSlot.Start.AsTime().In(time.Local).Format("2006-01-02 15:04:05"))
		t.Logf("结束时间: %s", nextSlot.End.AsTime().In(time.Local).Format("2006-01-02 15:04:05"))
	} else {
		t.Log("没有找到下一个时间段")
	}
}
