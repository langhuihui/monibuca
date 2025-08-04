package plugin_snap

import (
	m7s "m7s.live/v5"
	snap "m7s.live/v5/plugin/snap/pkg"
)

var _ = m7s.InstallPlugin[SnapPlugin](m7s.PluginMeta{
	NewTransformer: snap.NewTransform,
})

type SnapPlugin struct {
	m7s.Plugin
	QueryTimeDelta int `default:"3" desc:"查询截图时允许的最大时间差（秒）"`
}

// Start 在插件初始化时添加定时任务
func (p *SnapPlugin) Start() (err error) {
	// 初始化数据库
	if p.DB != nil {
		err = p.DB.AutoMigrate(&snap.SnapRecord{})
	}
	return
}
