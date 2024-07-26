package config

import (
	"testing"
)

// TestModify 测试动态修改配置文件，比较值是否修改，修改后是否有Modify属性
func TestModify(t *testing.T) {
	t.Run(t.Name(), func(t *testing.T) {
		var defaultValue struct {
			Subscribe
		}
		defaultValue.SubAudio = false
		var conf Config
		conf.Parse(&defaultValue)
		conf.ParseModifyFile(map[string]any{
			"subscribe": map[string]any{
				"subaudio": false,
			},
		})
		if conf.Modify != nil {
			t.Fail()
		}
		conf.ParseModifyFile(map[string]any{
			"subscribe": map[string]any{
				"subaudio": true,
			},
		})
		if conf.Modify == nil {
			t.Fail()
		}
	})
}

// TestGlobal 测试全局配置
func TestGlobal(t *testing.T) {
	t.Run(t.Name(), func(t *testing.T) {
		var defaultValue struct {
			Publish
		}
		var globalValue struct {
			Publish
		}
		globalValue.Publish.KickExist = true
		var conf Config
		var globalConf Config
		globalConf.Parse(&globalValue)
		conf.Parse(&defaultValue)
		conf.ParseGlobal(&globalConf)
		if defaultValue.Publish.KickExist != true {
			t.Fail()
		}
	})
}
