package main

import (
	. "github.com/langhuihui/monibuca/monica"
	_ "github.com/langhuihui/monibuca/plugins"
	"log"
	"os"
)

func main() {
	log.SetOutput(os.Stdout)
	Run("config.toml")
	select {}
}
