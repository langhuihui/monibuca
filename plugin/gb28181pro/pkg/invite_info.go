package gb28181

// InviteInfo 从INVITE消息中解析需要的信息
type InviteInfo struct {
	// 请求者ID
	RequesterId string `json:"requesterId"`
	// 目标通道ID
	TargetChannelId string `json:"targetChannelId"`
	// 源通道ID
	SourceChannelId string `json:"sourceChannelId"`
	// 会话名称
	SessionName string `json:"sessionName"`
	// SSRC
	SSRC string `json:"ssrc"`
	// 是否使用TCP
	TCP bool `json:"tcp"`
	// TCP是否为主动模式
	TCPActive bool `json:"tcpActive"`
	// 呼叫ID
	CallId string `json:"callId"`
	// 开始时间
	StartTime int64 `json:"startTime"`
	// 结束时间
	StopTime int64 `json:"stopTime"`
	// 下载速度
	DownloadSpeed string `json:"downloadSpeed"`
	// IP地址
	IP string `json:"ip"`
	// 端口
	Port int `json:"port"`
}

// NewInviteInfo 创建一个新的 InviteInfo 实例
func NewInviteInfo() *InviteInfo {
	return &InviteInfo{}
}
