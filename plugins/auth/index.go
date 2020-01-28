package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	. "github.com/langhuihui/monibuca/monica"
	"log"
	"strings"
	"time"
)

var (
	signs    = make(map[string]time.Time)
	signChan = make(chan string)
	config   = struct {
		Key string
	}{}
)

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "Auth",
		Type:   PLUGIN_HOOK,
		Config: &config,
		Run:    ClearSignCache,
	})
}

func onPublish(r *Room) {
	for _, v := range r.Subscribers {
		if err := CheckSign(v.Sign); err != nil {
			log.Printf("%s in room %s:%v", v.ID, r.StreamPath, err)
			v.Cancel()
		}
	}
}

// CheckSign 校验格式
func CheckSign(sign string) error {
	hexBytes, err := hex.DecodeString(sign)
	if err != nil {
		return fmt.Errorf("sign is not hex format %s", sign)
	}
	originString := string(decryptAES(hexBytes, []byte(config.Key)))
	if strings.HasPrefix(originString, config.Key) {
		if the_time, err := time.Parse("2006-01-02 15:04:05", originString[len(config.Key):]); err != nil {
			return err
		} else if time.Now().Sub(the_time).Hours() < 1 {
			return nil
		} else {
			return fmt.Errorf("sign has been overdue")
		}
	} else {
		return fmt.Errorf("sign does not HasPrefix %s", config.Key)
	}
}

// ClearSignCache 删除过期数据
func ClearSignCache() {
	AuthHooks.AddHook(CheckSign)
	OnPublishHooks.AddHook(onPublish)
	for {
		select {
		case now := <-time.After(time.Minute):
			for sign, t := range signs {
				if now.Sub(t).Hours() > 1 {
					delete(signs, sign)
				}
			}
		case sign := <-signChan:
			signs[sign] = time.Now()
		}
	}
}
func decryptAES(src []byte, key []byte) []byte {
	block, _ := aes.NewCipher(key)
	cipher.NewCBCDecrypter(block, key).CryptBlocks(src, src)
	n := len(src)
	return src[:n-int(src[n-1])]
}
