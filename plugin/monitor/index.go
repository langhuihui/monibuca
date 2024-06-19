package plugin_monitor

import (
	"github.com/polarsignals/frostdb"
	"m7s.live/m7s/v5"
)

var _ = m7s.InstallPlugin[MonitorPlugin]()

type MonitorPlugin struct {
	m7s.Plugin
	columnstore *frostdb.ColumnStore
}

func (cfg *MonitorPlugin) OnInit() (err error) {
	cfg.columnstore, err = frostdb.New()
	//database, _ := cfg.columnstore.DB(cfg, "monitor")
	if err != nil {
		return err
	}
	return
}
