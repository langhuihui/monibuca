package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/util"
	"github.com/langhuihui/monibuca/pm"
)

var instances = make(map[string]*pm.InstanceDesc)
var instancesDir string

func main() {

	println("start monibuca instance manager version:", Version)
	if MayBeError(readInstances()) {
		return
	}
	addr := flag.String("port", "8000", "http server port")
	flag.Parse()
	http.HandleFunc("/instance/listDir", listDir)
	http.HandleFunc("/instance/import", importInstance)
	http.HandleFunc("/instance/updateConfig", updateConfig)
	http.HandleFunc("/instance/list", listInstance)
	http.HandleFunc("/instance/create", initInstance)
	http.HandleFunc("/instance/restart", restartInstance)
	http.HandleFunc("/instance/shutdown", shutdownInstance)
	http.HandleFunc("/", website)
	fmt.Printf("start listen at %s", *addr)
	if err := http.ListenAndServe(":"+*addr, nil); err != nil {
		log.Fatal(err)
	}
}

func listDir(w http.ResponseWriter, r *http.Request) {
	if input := r.URL.Query().Get("input"); input != "" {
		if dir, err := os.Open(filepath.Dir(input)); err == nil {
			var dirs []string
			if infos, err := dir.Readdir(0); err == nil {
				for _, info := range infos {
					if info.IsDir() {
						dirs = append(dirs, info.Name())
					}
				}
				if bytes, err := json.Marshal(dirs); err == nil {
					w.Write(bytes)
				}
			}
		}
	}
}

func importInstance(w http.ResponseWriter, r *http.Request) {
	var e error
	defer func() {
		result := "success"
		if e != nil {
			result = e.Error()
		}
		w.Write([]byte(result))
	}()
	name := r.URL.Query().Get("name")
	if importPath := r.URL.Query().Get("path"); importPath != "" {
		if strings.HasSuffix(importPath, "/") {
			importPath = importPath[:len(importPath)-1]
		}
		f, err := os.Open(importPath)
		if e = err; err != nil {
			return
		}
		children, err := f.Readdir(0)
		if e = err; err == nil {
			var hasMain, hasConfig, hasMod, hasRestart bool
			for _, child := range children {
				switch child.Name() {
				case "main.go":
					hasMain = true
				case "config.toml":
					hasConfig = true
				case "go.mod":
					hasMod = true
				case "restart.sh", "restart.bat":
					hasRestart = true
				}
			}
			if hasMain && hasConfig && hasMod && hasRestart {
				if name == "" {
					_, name = path.Split(importPath)
				}
				config, err := ioutil.ReadFile(path.Join(importPath, "config.toml"))
				if e = err; err != nil {
					return
				}
				mainGo, err := ioutil.ReadFile(path.Join(importPath, "main.go"))
				if e = err; err != nil {
					return
				}
				reg, err := regexp.Compile("_ \"(.+)\"")
				if e = err; err != nil {
					return
				}
				instances[name] = &pm.InstanceDesc{
					Name:    name,
					Path:    importPath,
					Plugins: nil,
					Config:  string(config),
				}
				for _, m := range reg.FindAllStringSubmatch(string(mainGo), -1) {
					instances[name].Plugins = append(instances[name].Plugins, m[1])
				}
				var file *os.File
				file, e = os.OpenFile(path.Join(instancesDir, name+".toml"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
				if err != nil {
					return
				}
				tomlEncoder := toml.NewEncoder(file)
				e = tomlEncoder.Encode(instances[name])
			} else {
				e = errors.New("路径中缺少文件")
			}
		}
	} else {
		w.Write([]byte("参数错误"))
	}
}

func readInstances() error {
	if homeDir, err := Home(); err == nil {
		instancesDir = path.Join(homeDir, ".monibuca")
		if err = os.MkdirAll(instancesDir, os.FileMode(0666)); err == nil {
			if f, err := os.Open(instancesDir); err != nil {
				return err
			} else if cs, err := f.Readdir(0); err != nil {
				return err
			} else {
				for _, configFile := range cs {
					des := new(pm.InstanceDesc)
					if _, err = toml.DecodeFile(path.Join(instancesDir, configFile.Name()), des); err == nil {
						instances[des.Name] = des
					} else {
						log.Println(err)
					}
				}
				return nil
			}
		} else {
			return err
		}
	} else {
		return err
	}
}

func website(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Path
	if filePath == "/" {
		filePath = "/index.html"
	}
	if mime := mime.TypeByExtension(path.Ext(filePath)); mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	_, currentFilePath, _, _ := runtime.Caller(0)
	if f, err := ioutil.ReadFile(path.Join(path.Dir(currentFilePath), "pm/dist", filePath)); err == nil {
		if _, err = w.Write(f); err != nil {
			w.WriteHeader(505)
		}
	} else {
		w.Header().Set("Location", "/")
		w.WriteHeader(302)
	}
}
func listInstance(w http.ResponseWriter, r *http.Request) {
	if bytes, err := json.Marshal(instances); err == nil {
		_, err = w.Write(bytes)
	} else {
		w.Write([]byte(err.Error()))
	}
}
func initInstance(w http.ResponseWriter, r *http.Request) {
	instanceDesc := new(pm.InstanceDesc)
	sse := util.NewSSE(w, r.Context())
	err := json.Unmarshal([]byte(r.URL.Query().Get("info")), instanceDesc)
	clearDir := r.URL.Query().Get("clear") != ""
	defer func() {
		if err != nil {
			sse.WriteEvent("exception", []byte(err.Error()))
		} else {
			sse.Write([]byte("success"))
		}
	}()
	if err != nil {
		return
	}
	sse.WriteEvent("step", []byte("1:参数解析成功！"))
	err = instanceDesc.CreateDir(sse, clearDir)
	if err != nil {
		return
	}
	sse.WriteEvent("step", []byte("6:实例创建成功！"))
	var file *os.File
	file, err = os.OpenFile(path.Join(instancesDir, instanceDesc.Name+".toml"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return
	}
	tomlEncoder := toml.NewEncoder(file)
	err = tomlEncoder.Encode(&instanceDesc)
	if err != nil {
		return
	}
	instances[instanceDesc.Name] = instanceDesc
}
func shutdownInstance(w http.ResponseWriter, r *http.Request) {
	instanceName := r.URL.Query().Get("instance")
	if instance, ok := instances[instanceName]; ok {
		if err := instance.ShutDownCmd().Run(); err == nil {
			w.Write([]byte("success"))
		} else {
			w.Write([]byte(err.Error()))
		}
	} else {
		w.Write([]byte("no such instance"))
	}
}
func restartInstance(w http.ResponseWriter, r *http.Request) {
	sse := util.NewSSE(w, r.Context())
	instanceName := r.URL.Query().Get("instance")
	needUpdate := r.URL.Query().Get("update") != ""
	needBuild := r.URL.Query().Get("build") != ""
	if instance, ok := instances[instanceName]; ok {
		if needUpdate {
			if err := sse.WriteExec(instance.Command("go", "get", "-u")); err != nil {
				sse.WriteEvent("failed", []byte(err.Error()))
				return
			}
		}
		if needBuild {
			if err := sse.WriteExec(instance.Command("go", "build")); err != nil {
				sse.WriteEvent("failed", []byte(err.Error()))
				return
			}
		}
		if err := sse.WriteExec(instance.RestartCmd()); err != nil {
			sse.WriteEvent("failed", []byte(err.Error()))
			return
		}
		sse.Write([]byte("success"))
	} else {
		sse.WriteEvent("failed", []byte("no such instance"))
	}
}

func updateConfig(w http.ResponseWriter, r *http.Request) {
	instanceName := r.URL.Query().Get("instance")
	if instance, ok := instances[instanceName]; ok {
		f, err := os.OpenFile(path.Join(instance.Path, "config.toml"), os.O_WRONLY|os.O_TRUNC, 0666)
		if err != nil {
			w.Write([]byte(err.Error()))
			return
		}
		_, err = io.Copy(f, r.Body)
		if err != nil {
			w.Write([]byte(err.Error()))
			return
		}
		w.Write([]byte("success"))
	} else {
		w.Write([]byte("no such instance"))
	}
}
func Home() (string, error) {
	if user, err := user.Current(); nil == err {
		return user.HomeDir, nil
	}
	return pm.HomeDir()
}
