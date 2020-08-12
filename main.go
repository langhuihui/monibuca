package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	_ "github.com/Monibuca/clusterplugin"
	_ "github.com/Monibuca/gatewayplugin"
	_ "github.com/Monibuca/hdlplugin"
	_ "github.com/Monibuca/hlsplugin"
	_ "github.com/Monibuca/jessicaplugin"
	_ "github.com/Monibuca/logrotateplugin"
	_ "github.com/Monibuca/recordplugin"
	_ "github.com/Monibuca/rtmpplugin"
	_ "github.com/Monibuca/rtspplugin"
	_ "github.com/Monibuca/tsplugin"
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
	waiter(context.Background())
}

func waiter(ctx context.Context) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)
	<-sigc
	ctx.Done()
}
