package transcode

import "m7s.live/m7s/v5"

// / 定义传输模式的常量
const (
	TRANS_MODE_PIPE TransMode = "pipe"
	TRANS_MODE_RTSP TransMode = "rtsp"
	TRANS_MODE_RTMP TransMode = "rtmp"
	TRANS_MODE_LIB  TransMode = "lib"
)

type (
	TransMode    string
	DecodeConfig struct {
		Codec string `json:"codec" desc:"解码器"`
		Track string `json:"track" desc:"待解码的 track 名称"`
		Args  string `json:"args" desc:"解码参数"`
	}
	EncodeConfig struct {
		Codec string `json:"codec" desc:"编码器"`
		Track string `json:"track" desc:"待编码的 track 名称"`
		Args  string `json:"args" desc:"编码参数"`
		Dest  string `json:"dest" desc:"目标主机路径"`
	}
	TransRule struct {
		From      DecodeConfig   `json:"from"`
		To        []EncodeConfig `json:"to" desc:"编码配置"`            //目标
		Mode      TransMode      `json:"mode" desc:"转码模式"`          //转码模式
		LogToFile bool           `json:"logtofile" desc:"转码是否写入日志"` //转码日志写入文件
		PreStart  bool           `json:"prestart" desc:"是否预转码"`     //预转码
	}
)

func NewTransform() m7s.ITransformer {
	return &Transformer{}
}

type Transformer struct {
	m7s.DefaultTransformer
}

func (t *Transformer) Start() (err error) {
	err = t.TransformJob.Subscribe()
	if err == nil {

	}
	return
}
