package monica

import (
	"encoding/json"
	"github.com/BurntSushi/toml"
	"io/ioutil"
	"log"
)

var ConfigRaw []byte

func Run(configFile string) (err error) {
	if ConfigRaw, err = ioutil.ReadFile(configFile); err != nil {
		return
	}
	go Summary.StartSummary()
	if _, err = toml.Decode(string(ConfigRaw), cg); err == nil {
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
