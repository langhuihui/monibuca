package cascade

import (
	"time"

	"gorm.io/gorm"
)

// ServerRunRecord 服务器运行记录（数据库模型）
type ServerRunRecord struct {
	gorm.Model
	QuicAddr  string     // 服务器QUIC地址 (IP:Port)
	Priority  int        // 优先级 (1=主, 2=备)
	StartTime time.Time  // 服务器开始运行时间（连接成功时间）
	EndTime   *time.Time // 服务器结束运行时间（断开时间），nil表示当前在线
}

// {{ AURA-X: 实现 GetKey 接口，供 util.Collection 使用 }}
func (r *ServerRunRecord) GetKey() string {
	return r.QuicAddr
}
