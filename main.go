package main

import (
	_ "github.com/Monibuca/clusterplugin"
	. "github.com/Monibuca/engine"
	_ "github.com/Monibuca/gatewayplugin"
	_ "github.com/Monibuca/jessicaplugin"
	_ "github.com/Monibuca/logrotateplugin"
	_ "github.com/Monibuca/recordplugin"
	_ "github.com/Monibuca/rtmpplugin"
	_ "github.com/Monibuca/rtspplugin"
)

func main() {
	Run("config.toml")
	select {}
}