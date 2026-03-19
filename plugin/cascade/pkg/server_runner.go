package cascade

import (
	"time"

	"m7s.live/v5/pkg/util"

	"gorm.io/gorm"
)

// {{ AURA-X: M2 - 服务器运行状态追踪 }}
// ServerRunner 记录每台服务器的运行时间段（在线/离线），持久化到数据库

// ServerRunner 服务器运行状态追踪器
type ServerRunner struct {
	records util.Collection[string, *ServerRunRecord] // quicAddr -> 运行记录（内存缓存）
	db      *gorm.DB                                  // 数据库连接
}

// NewServerRunner 创建新的服务器运行追踪器
// {{ AURA-X: 将在 M3 的 FailoverController 中被调用 }}
func NewServerRunner(db *gorm.DB) *ServerRunner {
	sr := &ServerRunner{
		records: util.Collection[string, *ServerRunRecord]{},
		db:      db,
	}

	// {{ AURA-X: 自动迁移数据库表 }}
	sr.db.AutoMigrate(&ServerRunRecord{})

	// {{ AURA-X: 加载所有离线记录到内存（在线的只保留最新一条） }}
	sr.loadFromDB()

	return sr
}

// loadFromDB 从数据库加载记录到内存
// {{ AURA-X: 在线记录只保留最新的，离线记录全部加载 }}
func (sr *ServerRunner) loadFromDB() {
	var allRecords []ServerRunRecord
	sr.db.Order("start_time asc").Find(&allRecords)

	// 按 quicAddr 分组，只保留每个服务器的最新记录
	latestByServer := make(map[string]*ServerRunRecord)
	for i := range allRecords {
		record := &allRecords[i]
		// 如果记录还未结束（在线），只保留最新的
		if record.EndTime == nil {
			if existing, ok := latestByServer[record.QuicAddr]; !ok || existing.StartTime.Before(record.StartTime) {
				latestByServer[record.QuicAddr] = record
			}
		} else {
			// 离线记录全部加载
			sr.records.Set(record)
		}
	}

	// 添加在线记录
	for _, record := range latestByServer {
		sr.records.Set(record)
	}
}

// RecordStart 记录服务器开始运行（连接成功）
func (sr *ServerRunner) RecordStart(quicAddr string, priority int) {
	now := time.Now()

	// 如果已有记录且未结束，先结束它
	if existing, ok := sr.records.Get(quicAddr); ok && existing.EndTime == nil {
		existing.EndTime = &now
		// {{ AURA-X: 更新数据库 }}
		sr.db.Model(existing).Update("end_time", now)
	}

	// 创建新的运行记录
	record := &ServerRunRecord{
		QuicAddr:  quicAddr,
		Priority:  priority,
		StartTime: now,
		EndTime:   nil,
	}

	// {{ AURA-X: 保存到数据库 }}
	sr.db.Create(record)

	// {{ AURA-X: 添加到内存缓存 }}
	sr.records.Set(record)
}

// RecordStop 记录服务器停止运行（断开连接）
func (sr *ServerRunner) RecordStop(quicAddr string) {
	now := time.Now()

	// 更新现有记录
	if existing, ok := sr.records.Get(quicAddr); ok && existing.EndTime == nil {
		existing.EndTime = &now
		// {{ AURA-X: 更新数据库 }}
		sr.db.Model(existing).Update("end_time", now)
		// {{ AURA-X: 从内存缓存中移除（因为已离线） }}
		sr.records.RemoveByKey(quicAddr)
	}
}

// IsOnline 检查服务器是否在线
func (sr *ServerRunner) IsOnline(quicAddr string) bool {
	if record, ok := sr.records.Get(quicAddr); ok {
		return record.EndTime == nil
	}
	return false
}

// GetRecord 获取服务器的当前运行记录
func (sr *ServerRunner) GetRecord(quicAddr string) *ServerRunRecord {
	if record, ok := sr.records.Get(quicAddr); ok {
		return record
	}
	return nil
}

// GetAllRecords 获取所有历史运行记录
func (sr *ServerRunner) GetAllRecords() []ServerRunRecord {
	var records []ServerRunRecord
	sr.db.Order("start_time desc").Find(&records)
	return records
}

// GetOnlineServers 获取当前所有在线服务器地址
func (sr *ServerRunner) GetOnlineServers() []string {
	var online []string
	sr.records.Range(func(record *ServerRunRecord) bool {
		if record.EndTime == nil {
			online = append(online, record.QuicAddr)
		}
		return true
	})
	return online
}

// QueryByTimeRange 查询指定时间范围内的运行记录
func (sr *ServerRunner) QueryByTimeRange(start, end time.Time) []ServerRunRecord {
	var records []ServerRunRecord
	sr.db.Where("start_time >= ? AND start_time <= ?", start, end).Order("start_time asc").Find(&records)
	return records
}

// GetServerHistory 获取指定服务器的所有历史记录
func (sr *ServerRunner) GetServerHistory(quicAddr string) []ServerRunRecord {
	var records []ServerRunRecord
	sr.db.Where("quic_addr = ?", quicAddr).Order("start_time desc").Find(&records)
	return records
}

// Clear 清除所有运行记录（仅用于测试）
func (sr *ServerRunner) Clear() {
	sr.db.Where("1=1").Delete(&ServerRunRecord{})
	sr.records = util.Collection[string, *ServerRunRecord]{}
}
