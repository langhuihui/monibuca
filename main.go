package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/util"
)

type InstanceDesc struct {
	Name    string
	Path    string
	Plugins []string
	Config  string
}

var instances = make(map[string]*InstanceDesc)
var instancesDir string

func main() {
	// log.SetOutput(os.Stdout)
	// configPath := flag.String("c", "config.toml", "configFile")
	// flag.Parse()
	// Run(*configPath)
	// select {}
	println("start monibuca instance manager version:", Version)
	if MayBeError(readInstances()) {
		return
	}
	addr := flag.String("port", "8000", "http server port")
	flag.Parse()

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
					des := new(InstanceDesc)
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
	instanceDesc := new(InstanceDesc)
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
	err = instanceDesc.createDir(sse, clearDir)
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
		if err := instance.command("kill", "-9", "`cat pid`").Run(); err == nil {
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
			if err := instance.writeExecSSE(sse, exec.Command("go", "get", "-u")); err != nil {
				sse.WriteEvent("failed", []byte(err.Error()))
				return
			}
		}
		if needBuild {
			if err := instance.writeExecSSE(sse, exec.Command("go", "build")); err != nil {
				sse.WriteEvent("failed", []byte(err.Error()))
				return
			}
		}
		if err := instance.writeExecSSE(sse, exec.Command("sh", "restart.sh")); err != nil {
			sse.WriteEvent("failed", []byte(err.Error()))
			return
		}
		sse.Write([]byte("success"))
	} else {
		sse.WriteEvent("failed", []byte("no such instance"))
	}
}
func (p *InstanceDesc) writeExecSSE(sse *util.SSE, cmd *exec.Cmd) error {
	cmd.Dir = p.Path
	return sse.WriteExec(cmd)
}
func (p *InstanceDesc) command(name string, args ...string) (cmd *exec.Cmd) {
	cmd = exec.Command(name, args...)
	cmd.Dir = p.Path
	return
}
func (p *InstanceDesc) createDir(sse *util.SSE, clearDir bool) (err error) {
	if clearDir {
		os.RemoveAll(p.Path)
	}
	err = os.MkdirAll(p.Path, 0666)
	if err != nil {
		return
	}
	sse.WriteEvent("step", []byte("2:目录创建成功！"))
	err = ioutil.WriteFile(path.Join(p.Path, "config.toml"), []byte(p.Config), 0666)
	if err != nil {
		return
	}
	var build bytes.Buffer
	build.WriteString(`package main
import(
"github.com/langhuihui/monibuca/monica"`)
	for _, plugin := range p.Plugins {
		build.WriteString("\n_ \"")
		build.WriteString(plugin)
		build.WriteString("\"")
	}
	build.WriteString("\n)\n")
	build.WriteString(`
func main(){
	monica.Run("config.toml")
	select{}
}
`)
	err = ioutil.WriteFile(path.Join(p.Path, "main.go"), build.Bytes(), 0666)
	if err != nil {
		return
	}
	sse.WriteEvent("step", []byte("3:文件创建成功！"))
	err = p.writeExecSSE(sse, exec.Command("go", "mod", "init", p.Name))
	if err != nil {
		return
	}
	sse.WriteEvent("step", []byte("4:go mod 初始化完成！"))
	err = p.writeExecSSE(sse, exec.Command("go", "build"))
	if err != nil {
		return
	}
	sse.WriteEvent("step", []byte("5:go build 成功！"))
	build.Reset()
	build.WriteString("kill -9 `cat pid`\nnohup ./")
	binFile := strings.TrimSuffix(p.Path, "/")
	_, binFile = path.Split(binFile)
	build.WriteString(binFile)
	build.WriteString(" > log.txt & echo $! > pid\n")
	err = ioutil.WriteFile(path.Join(p.Path, "restart.sh"), build.Bytes(), 0777)
	if err != nil {
		return
	}
	return p.writeExecSSE(sse, exec.Command("sh", "restart.sh"))
}
func Home() (string, error) {
	user, err := user.Current()
	if nil == err {
		return user.HomeDir, nil
	}

	// cross compile support

	if "windows" == runtime.GOOS {
		return homeWindows()
	}

	// Unix-like system, so just assume Unix
	return homeUnix()
}

func homeUnix() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	// If that fails, try the shell
	var stdout bytes.Buffer
	cmd := exec.Command("sh", "-c", "eval echo ~$USER")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", errors.New("blank output when reading home directory")
	}

	return result, nil
}

func homeWindows() (string, error) {
	drive := os.Getenv("HOMEDRIVE")
	path := os.Getenv("HOMEPATH")
	home := drive + path
	if drive == "" || path == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		return "", errors.New("HOMEDRIVE, HOMEPATH, and USERPROFILE are blank")
	}

	return home, nil
}
