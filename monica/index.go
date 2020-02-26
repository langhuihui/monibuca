package monica

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"
)

var ConfigRaw []byte
var Version = "0.4.0"
var EngineInfo = &struct {
	Version   string
	StartTime time.Time
}{Version, time.Now()}

func Run(configFile string) (err error) {
	if runtime.GOOS == "windows" {
		ioutil.WriteFile("shutdown.bat", []byte(fmt.Sprintf("taskkill /pid %d  -t  -f", os.Getpid())), 0777)
	} else {
		ioutil.WriteFile("shutdown.sh", []byte(fmt.Sprintf("kill -9 %d", os.Getpid())), 0777)
	}
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
