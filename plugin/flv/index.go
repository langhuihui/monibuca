package plugin_flv

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/flv/pb"
	. "m7s.live/v5/plugin/flv/pkg"
)

type FLVPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	Path string
}

const defaultConfig m7s.DefaultYaml = `publish:
  speed: 1`

var _ = m7s.InstallPlugin[FLVPlugin](m7s.PluginMeta{
	DefaultYaml:         defaultConfig,
	NewPuller:           NewPuller,
	NewRecorder:         NewRecorder,
	RegisterGRPCHandler: pb.RegisterApiHandler,
	ServiceDesc:         &pb.Api_ServiceDesc,
	NewPullProxy:        m7s.NewHTTPPullPorxy,
})

func (plugin *FLVPlugin) Start() (err error) {
	_, port, _ := strings.Cut(plugin.GetCommonConf().HTTP.ListenAddr, ":")
	if port == "80" {
		plugin.PlayAddr = append(plugin.PlayAddr, "http://{hostName}/flv/{streamPath}", "ws://{hostName}/flv/{streamPath}")
	} else if port != "" {
		plugin.PlayAddr = append(plugin.PlayAddr, fmt.Sprintf("http://{hostName}:%s/flv/{streamPath}", port), fmt.Sprintf("ws://{hostName}:%s/flv/{streamPath}", port))
	}
	_, port, _ = strings.Cut(plugin.GetCommonConf().HTTP.ListenAddrTLS, ":")
	if port == "443" {
		plugin.PlayAddr = append(plugin.PlayAddr, "https://{hostName}/flv/{streamPath}", "wss://{hostName}/flv/{streamPath}")
	} else if port != "" {
		plugin.PlayAddr = append(plugin.PlayAddr, fmt.Sprintf("https://{hostName}:%s/flv/{streamPath}", port), fmt.Sprintf("wss://{hostName}:%s/flv/{streamPath}", port))
	}
	return
}

func (plugin *FLVPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	streamPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".flv")
	var err error
	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}()
	var live Live
	if r.URL.RawQuery != "" {
		streamPath += "?" + r.URL.RawQuery
	}
	live.Subscriber, err = plugin.Subscribe(r.Context(), streamPath)
	if err != nil {
		return
	}
	live.Subscriber.RemoteAddr = r.RemoteAddr

	var ctx util.HTTP_WS_Writer
	ctx.Conn, err = live.Subscriber.CheckWebSocket(w, r)
	if err != nil {
		return
	}
	ctx.WriteTimeout = plugin.GetCommonConf().WriteTimeout
	ctx.ContentType = "video/x-flv"
	ctx.ServeHTTP(w, r)
	live.WriteFlvTag = func(flv net.Buffers) (err error) {
		_, err = flv.WriteTo(&ctx)
		if err != nil {
			return
		}
		return ctx.Flush()
	}
	err = live.Run()
}
