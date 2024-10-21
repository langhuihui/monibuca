package transcode

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	flv "m7s.live/v5/plugin/flv/pkg"
)

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
		Mode  TransMode `default:"pipe" json:"mode" desc:"转码模式"` //转码模式
		Codec string    `json:"codec" desc:"解码器"`
		Args  string    `json:"args" desc:"解码参数"`
	}
	EncodeConfig struct {
		Codec string `json:"codec" desc:"编码器"`
		Args  string `json:"args" desc:"编码参数"`
		Dest  string `json:"dest" desc:"目标主机路径"`
	}
	TransRule struct {
		From      DecodeConfig   `json:"from"`
		To        []EncodeConfig `json:"to" desc:"编码配置"`            //目标
		LogToFile bool           `json:"logtofile" desc:"转码是否写入日志"` //转码日志写入文件
		PreStart  bool           `json:"prestart" desc:"是否预转码"`     //预转码
	}
)

func NewTransform() m7s.ITransformer {
	ret := &Transformer{}
	ret.SetDescription(task.OwnerTypeKey, "Transcode")
	return ret
}

type Transformer struct {
	m7s.DefaultTransformer
	TransRule
	logFileName string
	logFile     *os.File
	ffmpeg      *exec.Cmd
}

func (t *Transformer) Start() (err error) {
	if t.TransformJob.Config.Input != nil {
		switch v := t.TransformJob.Config.Input.(type) {
		case DecodeConfig:
			t.From = v
		case map[string]any:
			config.Parse(&t.TransRule.From, v)
		case string:
			t.From.Mode = TRANS_MODE_PIPE
			t.From.Args = v
		}
	}
	if t.From.Mode == "" {
		t.From.Mode = TRANS_MODE_PIPE
	}
	args := strings.Fields(t.From.Args)
	if t.From.Codec != "" {
		args = append(args, "-c:v", t.From.Codec)
	}
	switch t.From.Mode {
	case "pipe":
		err = t.TransformJob.Subscribe()
		if err != nil {
			return
		}
		args = append(args, "-f", "flv", "-i", "pipe:0")
	case "rtsp":
		if rtspPlugin, ok := t.TransformJob.Plugin.Server.Plugins.Get("RTSP"); ok {
			listenAddr := rtspPlugin.GetCommonConf().TCP.ListenAddr
			if strings.HasPrefix(listenAddr, ":") {
				listenAddr = "localhost" + listenAddr
			}
			args = append(args, "-i", "rtsp://"+listenAddr+"/"+t.TransformJob.StreamPath)
		}
	case "rtmp":
		if rtmpPlugin, ok := t.TransformJob.Plugin.Server.Plugins.Get("RTMP"); ok {
			listenAddr := rtmpPlugin.GetCommonConf().TCP.ListenAddr
			if strings.HasPrefix(listenAddr, ":") {
				listenAddr = "localhost" + listenAddr
			}
			args = append(args, "-i", "rtmp://"+listenAddr+"/"+t.TransformJob.StreamPath)
		}
	}
	t.To = make([]EncodeConfig, len(t.TransformJob.Config.Output))
	for i, to := range t.TransformJob.Config.Output {
		var enc EncodeConfig
		if to.Conf != nil {
			switch v := to.Conf.(type) {
			case map[string]any:
				config.Parse(&enc, v)
			case string:
				enc.Args = v
			}
		}
		t.To[i] = enc
		//if to.Overlay != "" {
		//	args = append(args, "-i", to.Overlay)
		//}
		//if to.Filter != "" {
		//	args = append(args, "-filter_complex", strings.ReplaceAll(to.Filter, "\n", ""))
		//	args = append(args, "-map", "[out]")
		//	args = append(args, "-map", "0:a")
		//}
		args = append(args, strings.Fields(enc.Args)...)
		var targetUrl *url.URL
		targetUrl, err = url.Parse(to.Target)
		if err != nil {
			return
		}
		switch targetUrl.Scheme {
		case "rtmp":
			args = append(args, "-f", "flv", to.Target)
		case "rtsp":
			args = append(args, "-f", "rtsp", to.Target)
		case "srt":
			args = append(args, "-f", "mpegts", to.Target)
		default:
			args = append(args, to.Target)
		}
	}
	t.SetDescription("cmd", args)
	t.SetDescription("config", t.TransRule)
	//t.BufReader.Dump, err = os.OpenFile("dump.flv", os.O_CREATE|os.O_WRONLY, 0644)
	t.logFileName = fmt.Sprintf("logs/transcode_%s_%s.log", strings.ReplaceAll(t.TransformJob.StreamPath, "/", "_"), time.Now().Format("20060102150405"))
	t.ffmpeg = exec.CommandContext(t, "ffmpeg", args...)
	if t.logFileName != "" {
		t.SetDescription("log", t.logFileName)
		t.logFile, err = os.OpenFile(t.logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			t.Error("Could not create transcode log", "err", err)
			return err
		}
		// 将命令的标准输出和标准错误输出重定向到日志文件
		t.ffmpeg.Stdout = t.logFile
		t.ffmpeg.Stderr = t.logFile

	} else {
		// 将命令的标准输出和标准错误输出重定向到操作系统的标准输出和标准错误输出
		t.ffmpeg.Stdout = os.Stdout
		t.ffmpeg.Stderr = os.Stderr
	}
	t.Info("start exec", "cmd", t.ffmpeg.String())
	return t.ffmpeg.Start()
}

func (t *Transformer) Run() error {
	t.SetDescription("pid", t.ffmpeg.Process.Pid)
	if t.From.Mode == "pipe" {
		rBuf := make(chan []byte, 100)
		t.ffmpeg.Stdin = util.NewBufReaderChan(rBuf)
		var live flv.Live
		live.Subscriber = t.TransformJob.Subscriber
		var bufferFull time.Time
		live.WriteFlvTag = func(flv net.Buffers) (err error) {
			var buffer []byte
			for _, b := range flv {
				buffer = append(buffer, b...)
			}
			select {
			case rBuf <- buffer:
				bufferFull = time.Now()
			default:
				t.Warn("pipe input buffer full")
				if time.Since(bufferFull) > time.Second*5 {
					t.Stop(bufio.ErrBufferFull)
				}
			}
			return
		}
		defer close(rBuf)
		return live.Run()
	} else {
		return t.ffmpeg.Wait()
	}
}

func (t *Transformer) Dispose() {
	err := t.ffmpeg.Process.Kill()
	t.Error("kill ffmpeg", "err", err)
	if t.logFile != nil {
		_ = t.logFile.Close()
	}
}
