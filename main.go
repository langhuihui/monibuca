package main

import (
	"flag"
	. "github.com/langhuihui/monibuca/monica"
	_ "github.com/langhuihui/monibuca/plugins"
	"log"
	"os"
)

func main() {
	log.SetOutput(os.Stdout)
	configPath := flag.String("c", "config.toml", "configFile")
	flag.Parse()
	Run(*configPath)
	select {}
}
