package pkg

type Device interface {
	Login() (int, error)
	LoginV4() (int, error)
	GetDeiceInfo() (*DeviceInfo, error)
	GetChannelName() (map[int]string, error)
	Logout() error
	SetAlarmCallBack() error
	StartListenAlarmMsg() error
	StopListenAlarmMsg() error
	PTZControlWithSpeed(dwPTZCommand, dwStop, dwSpeed int) (bool, error)
	PTZControlWithSpeed_Other(lChannel, dwPTZCommand, dwStop, dwSpeed int) (bool, error)
	PTZControl(dwPTZCommand, dwStop int) (bool, error)
	PTZControl_Other(lChannel, dwPTZCommand, dwStop int) (bool, error)
	GetChannelPTZ(channel int)
	RealPlay_V40(ChannelId int,receiver *Receiver) (int, error)
	StopRealPlay()
}
type DeviceInfo struct {
	IP         string
	Port       int
	UserName   string
	Password   string
	DeviceID   string //序列号
	DeviceName string //DVR名称
	ByChanNum  int    //通道数量
}
