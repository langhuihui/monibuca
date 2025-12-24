package plugin_crontab

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// 构造 168 位计划字符串的辅助方法，hours 是一周内按小时索引的开启位置（0=周日0点）。
func buildPlanWithOnHours(hours ...int) string {
	const total = 168
	b := make([]byte, total)
	for i := 0; i < total; i++ {
		b[i] = '0'
	}
	for _, h := range hours {
		if h >= 0 && h < total {
			b[h] = '1'
		}
	}
	return string(b)
}

// ============================
// nextSlotRange 行为测试
// ============================

// 测试：同一天内的连续时间段计算
func TestNextSlotRange_SameDay(t *testing.T) {
	// 周一 10:00-12:00 开启（即 10:00-11:00、11:00-12:00 两个小时）
	plan := buildPlanWithOnHours(1*24+10, 1*24+11)
	loc := time.Local

	// 周一 09:30 -> 期望下一个时间段是 10:00-12:00
	now := time.Date(2023, 5, 1, 9, 30, 0, 0, loc) // 2023-05-01 是周一
	start, end, ok := nextSlotRange(plan, now, loc)
	require.True(t, ok)
	assert.Equal(t, time.Date(2023, 5, 1, 10, 0, 0, 0, loc), start)
	assert.Equal(t, time.Date(2023, 5, 1, 12, 0, 0, 0, loc), end)

	// 周一 10:30 -> 仍应返回同一时间段 10:00-12:00
	now2 := time.Date(2023, 5, 1, 10, 30, 0, 0, loc)
	start2, end2, ok2 := nextSlotRange(plan, now2, loc)
	require.True(t, ok2)
	assert.Equal(t, start, start2)
	assert.Equal(t, end, end2)
}

// 测试：跨天连续时间段（例如周一 22:00 到 周二 01:00），应视为一个连续时间段
func TestNextSlotRange_CrossDay(t *testing.T) {
	// 周一 22:00-23:00、23:00-00:00，周二 00:00-01:00 全为 1
	// 周日=0, 周一=1, 周二=2
	hours := []int{
		1*24 + 22, // 周一 22:00-23:00
		1*24 + 23, // 周一 23:00-00:00
		2 * 24,    // 周二 00:00-01:00
	}
	plan := buildPlanWithOnHours(hours...)
	loc := time.Local

	// 周一 21:30，期望下一个时间段为 周一 22:00 ~ 周二 01:00
	now := time.Date(2023, 5, 1, 21, 30, 0, 0, loc) // 2023-05-01 周一
	start, end, ok := nextSlotRange(plan, now, loc)
	require.True(t, ok)
	assert.Equal(t, time.Date(2023, 5, 1, 22, 0, 0, 0, loc), start)
	assert.Equal(t, time.Date(2023, 5, 2, 1, 0, 0, 0, loc), end)

	// 周一 23:30，仍应认为处于同一个跨天时间段
	now2 := time.Date(2023, 5, 1, 23, 30, 0, 0, loc)
	start2, end2, ok2 := nextSlotRange(plan, now2, loc)
	require.True(t, ok2)
	assert.Equal(t, start, start2)
	assert.Equal(t, end, end2)
}

// 测试：使用自定义 168 位计划字符串，方便手动验证具体案例。
// 你可以根据需要修改 plan 字符串和 now 时间，观察 nextSlotRange 的实际输出。
func TestNextSlotRange_Custom(t *testing.T) {
	// TODO: 在这里把 plan 替换成你要测试的 168 位字符串
	plan := "111111110000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000111111111111111111111111111111111111111111111111111111111"
	loc := time.Local

	// TODO: 在这里设置你要测试的当前时间
	now := time.Date(2025, 12, 18, 0, 0, 0, 0, loc)

	start, end, ok := nextSlotRange(plan, now, loc)
	t.Logf("plan len=%d, now=%s", len(plan), now.Format("2006-01-02 15:04:05"))
	if !ok {
		t.Log("未找到下一个时间段")
		return
	}

	t.Logf("下一个时间段: %s ~ %s",
		start.Format("2006-01-02 15:04:05"),
		end.Format("2006-01-02 15:04:05"),
	)
}
