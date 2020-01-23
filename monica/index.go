package monica

import (
	"encoding/json"
	"github.com/BurntSushi/toml"
	"log"
)

func Run(configFile string) (err error) {
	if _, err = toml.DecodeFile(configFile, cg); err == nil {
		for name, config := range plugins {
			if cfg, ok := cg.Plugins[name]; ok {
				b, _ := json.Marshal(cfg)
				if err = json.Unmarshal(b, config.Config); err != nil {
					log.Println(err)
					continue
				}
			} else if config.Config != nil {
				continue
			}
			if config.Run != nil {
				go config.Run()
			}
		}
	}
	return
}
