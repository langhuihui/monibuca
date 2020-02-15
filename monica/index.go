package monica

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"time"

	"github.com/BurntSushi/toml"
)

var ConfigRaw []byte
var Version = "0.2.6"
var EngineInfo = &struct {
	Version   string
	StartTime time.Time
}{Version, time.Now()}

func Run(configFile string) (err error) {
	log.Printf("start monibuca version:%s", Version)
	if ConfigRaw, err = ioutil.ReadFile(configFile); err != nil {
		log.Printf("read config file error: %v", err)
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
	} else {
		log.Printf("decode config file error: %v", err)
	}
	return
}
