//go:build ignore

package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

var debugShim string = `package main

import "net/http"

func init() {

	notSupport := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not support"))
	}

	http.HandleFunc("/debug/charts/", notSupport)
	http.HandleFunc("/debug/charts/data", notSupport)
	http.HandleFunc("/debug/charts/data-feed", notSupport)
}
`
var debug string = `package main

import (
	_ "github.com/mkevac/debugcharts"
)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run gen.go <path>")
		os.Exit(1)
	}
	var content string
	if os.Args[1] == "1" {
		content = debug
	} else {
		content = debugShim
	}
	ioutil.WriteFile("debug.go", []byte(content), 0666)
}
