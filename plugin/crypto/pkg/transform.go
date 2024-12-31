package crypto

import (
	"github.com/deepch/vdk/codec/h265parser"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/task"

	"fmt"

	m7s "m7s.live/v5"
	"m7s.live/v5/plugin/crypto/pkg/method"
)

// GlobalConfig 全局加密配置
var GlobalConfig Config

type Config struct {
	IsStatic   bool   `desc:"是否静态密钥" default:"false"`
	Algo       string `desc:"加密算法" default:"aes_ctr"` //加密算法
	EncryptLen int    `desc:"加密字节长度" default:"1024"`  //加密字节长度
	Secret     struct {
		Key string `desc:"加密密钥" default:"your key"` //加密密钥
		Iv  string `desc:"加密向量" default:"your iv"`  //加密向量
	} `desc:"密钥配置"`
}

type Transform struct {
	m7s.DefaultTransformer
	cryptor method.ICryptor
}

func NewTransform() m7s.ITransformer {
	ret := &Transform{}
	ret.SetDescription(task.OwnerTypeKey, "Crypto")
	return ret
}

// ValidateAndCreateKey 验证并创建加密密钥
func ValidateAndCreateKey(isStatic bool, algo string, secretKey, secretIv, streamPath string) (keyConf method.Key, err error) {
	if isStatic {
		switch algo {
		case "aes_ctr":
			keyConf.Key = secretKey
			keyConf.Iv = secretIv
			if len(keyConf.Iv) != 16 || len(keyConf.Key) != 32 {
				return keyConf, fmt.Errorf("key or iv length is wrong")
			}
		case "xor_s":
			keyConf.Key = secretKey
			if len(keyConf.Key) != 32 {
				return keyConf, fmt.Errorf("key length is wrong")
			}
		case "xor_c":
			keyConf.Key = secretKey
			keyConf.Iv = secretIv
			if len(keyConf.Iv) != 16 || len(keyConf.Key) != 32 {
				return keyConf, fmt.Errorf("key or iv length is wrong")
			}
		default:
			return keyConf, fmt.Errorf("algo type is wrong")
		}
	} else {
		/*
			动态加密
			key = md5(密钥+流名称)
			iv = md5(流名称）前一半
		*/
		if secretKey != "" {
			keyConf.Key = method.Md5Sum(secretKey + streamPath)
			keyConf.Iv = method.Md5Sum(streamPath)[:16]
		} else {
			return keyConf, fmt.Errorf("secret key is empty")
		}
	}
	return
}

func (t *Transform) Start() error {
	// 在 Start 时获取并保存配置
	t.Info("transform job started")

	keyConf, err := ValidateAndCreateKey(GlobalConfig.IsStatic, GlobalConfig.Algo, GlobalConfig.Secret.Key, GlobalConfig.Secret.Iv, t.TransformJob.StreamPath)
	if err != nil {
		return err
	}

	t.cryptor, err = method.GetCryptor(GlobalConfig.Algo, keyConf)
	if err != nil {
		t.Error("failed to create cryptor", "error", err)
		return err
	}

	// 使用 TransformJob 的 Subscribe 方法订阅流
	if err := t.TransformJob.Subscribe(); err != nil {
		t.Error("failed to subscribe stream", "error", err)
		return err
	}

	t.Info("crypto transform started",
		"stream", t.TransformJob.StreamPath,
		"algo", GlobalConfig.Algo,
		"isStatic", GlobalConfig.IsStatic,
	)

	return nil
}

func (t *Transform) Go() error {
	// 创建发布者
	if err := t.TransformJob.Publish(t.TransformJob.StreamPath + "/crypto"); err != nil {
		t.Error("failed to create publisher", "error", err)
		return err
	}

	// 处理音视频流
	return m7s.PlayBlock(t.TransformJob.Subscriber,
		func(audio *pkg.RawAudio) (err error) {
			copyAudio := &pkg.RawAudio{
				FourCC:    audio.FourCC,
				Timestamp: audio.Timestamp,
			}
			audio.Memory.Range(func(b []byte) {
				copy(copyAudio.NextN(len(b)), b)
			})
			return t.TransformJob.Publisher.WriteAudio(copyAudio)
		},
		func(video *pkg.H26xFrame) error {
			// 处理视频帧
			if video.GetSize() == 0 {
				return nil
			}
			copyVideo := &pkg.H26xFrame{
				FourCC:    video.FourCC,
				CTS:       video.CTS,
				Timestamp: video.Timestamp,
			}

			for _, nalu := range video.Nalus {
				mem := copyVideo.NextN(nalu.Size)
				copy(mem, nalu.ToBytes())
				needEncrypt := false
				if video.FourCC == codec.FourCC_H264 {
					switch codec.ParseH264NALUType(mem[0]) {
					case codec.NALU_Non_IDR_Picture, codec.NALU_IDR_Picture:
						needEncrypt = true
					}
				} else if video.FourCC == codec.FourCC_H265 {
					switch codec.ParseH265NALUType(mem[0]) {
					case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
						h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
						h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
						h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
						h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
						h265parser.NAL_UNIT_CODED_SLICE_CRA:
						needEncrypt = true
					}
				}
				if needEncrypt {
					if encBytes, err := t.cryptor.Encrypt(mem); err == nil {
						copyVideo.Nalus.Append(encBytes)
					} else {
						copyVideo.Nalus.Append(mem)
					}
				} else {
					copyVideo.Nalus.Append(mem)
				}
			}
			return t.TransformJob.Publisher.WriteVideo(copyVideo)
		})
}

func (t *Transform) Dispose() {
	t.Info("crypto transform disposed",
		"stream", t.TransformJob.StreamPath,
	)
}
