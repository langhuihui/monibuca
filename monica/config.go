package monica

import "log"

const (
	PLUGIN_SUBSCRIBER = 1
	PLUGIN_PUBLISHER  = 1 << 1
	PLUGIN_HOOK       = 1 << 2
)

var (
	cg      = &Config{Plugins: make(map[string]interface{})}
	plugins = make(map[string]*PluginConfig)
)

type PluginConfig struct {
	Name   string      //插件名称
	Type   byte        //类型
	Config interface{} //插件配置
	Run    func()
}

type Config struct {
	Plugins map[string]interface{}
}

func InstallPlugin(opt *PluginConfig) {
	log.Printf("install plugin %s", opt.Name)
	plugins[opt.Name] = opt
}

type ListenerConfig struct {
	ListenAddr string
}
