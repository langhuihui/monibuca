package crypto

import (
	"github.com/deepch/vdk/codec/h265parser"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/format"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"

	"fmt"

	m7s "m7s.live/v5"
	"m7s.live/v5/plugin/crypto/pkg/method"
)

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
	Writer  *m7s.PublishWriter[*format.RawAudio, *format.H26xFrame]
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
	if len(t.TransformJob.Config.Output) == 0 {
		return fmt.Errorf("output is empty")
	}
	output := t.TransformJob.Config.Output[0] // TODO: multiple output
	var cryptoConfig Config
	var conf config.Config
	conf.Parse(&cryptoConfig)
	conf.ParseModifyFile(output.Conf.(map[string]any))
	keyConf, err := ValidateAndCreateKey(cryptoConfig.IsStatic, cryptoConfig.Algo, cryptoConfig.Secret.Key, cryptoConfig.Secret.Iv, t.TransformJob.StreamPath)
	if err != nil {
		return err
	}

	t.cryptor, err = method.GetCryptor(cryptoConfig.Algo, keyConf)
	if err != nil {
		t.Error("failed to create cryptor", "error", err)
		return err
	}

	// 使用 TransformJob 的 Subscribe 方法订阅流
	if err := t.TransformJob.Subscribe(); err != nil {
		t.Error("failed to subscribe stream", "error", err)
		return err
	}
	t.SetDescription("algo", cryptoConfig.Algo)
	t.SetDescription("isStatic", cryptoConfig.IsStatic)

	// 创建发布者
	if err := t.TransformJob.Publish(output.StreamPath); err != nil {
		t.Error("failed to create publisher", "error", err)
		return err
	}
	t.TransformJob.Publisher.SetDescription("key", keyConf.Key)
	t.TransformJob.Publisher.SetDescription("iv", keyConf.Iv)
	return nil
}

func (t *Transform) Go() error {
	allocator := util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	defer allocator.Recycle()
	writer := m7s.NewPublisherWriter[*format.RawAudio, *format.H26xFrame](t.TransformJob.Publisher, allocator)
	// 处理音视频流
	return m7s.PlayBlock(t.TransformJob.Subscriber,
		func(audio *format.RawAudio) (err error) {
			copyAudio := writer.AudioFrame
			copyAudio.ICodecCtx = audio.ICodecCtx
			*writer.AudioFrame.BaseSample = *audio.BaseSample
			audio.CopyTo(copyAudio.NextN(audio.Size))
			return writer.NextAudio()
		},
		func(video *format.H26xFrame) error {
			copyVideo := writer.VideoFrame
			copyVideo.ICodecCtx = video.ICodecCtx
			*copyVideo.BaseSample = *video.BaseSample
			nalus := copyVideo.GetNalus()
			for nalu := range video.Raw.(*pkg.Nalus).RangePoint {
				p := nalus.GetNextPointer()
				mem := copyVideo.NextN(nalu.Size)
				nalu.CopyTo(mem)
				needEncrypt := false
				if video.FourCC() == codec.FourCC_H264 {
					switch codec.ParseH264NALUType(mem[0]) {
					case codec.NALU_Non_IDR_Picture, codec.NALU_IDR_Picture:
						needEncrypt = true
					}
				} else if video.FourCC() == codec.FourCC_H265 {
					switch codec.ParseH265NALUType(mem[0]) {
					case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
						h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
						h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
						h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
						h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
						h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
						h265parser.NAL_UNIT_CODED_SLICE_CRA:
						needEncrypt = true
					}
				}
				if needEncrypt {
					encBytes, err := t.cryptor.Encrypt(mem[2:])
					if err == nil {
						p.Push(mem[:2], encBytes)
					} else {
						p.PushOne(mem)
					}
				} else {
					p.PushOne(mem)
				}
			}
			return writer.NextVideo()
		})
}

func (t *Transform) Dispose() {
	t.Info("crypto transform disposed",
		"stream", t.TransformJob.StreamPath,
	)
}
