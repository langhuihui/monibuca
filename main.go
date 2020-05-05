package main

import (
	"flag"
	"path/filepath"
	"runtime"

	. "github.com/Monibuca/engine/v2"
	// _ "github.com/Monibuca/plugin-cluster"
	_ "github.com/Monibuca/plugin-gateway"
	_ "github.com/Monibuca/plugin-hdl"
	_ "github.com/Monibuca/plugin-hls"
	_ "github.com/Monibuca/plugin-jessica"
	_ "github.com/Monibuca/plugin-logrotate"
	_ "github.com/Monibuca/plugin-record"
	_ "github.com/Monibuca/plugin-rtmp"
	_ "github.com/Monibuca/plugin-rtsp"
	_ "github.com/Monibuca/plugin-ts"
)

func main() {
	addr := flag.String("c", "", "config file")
	flag.Parse()
	if *addr == "" {
		_, currentFile, _, _ := runtime.Caller(0)
		configFIle := filepath.Join(filepath.Dir(currentFile), "config.toml")
		Run(configFIle)
	} else {
		Run(*addr)
	}
	select {}
}
