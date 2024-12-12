package plugin_snap

import (
	m7s "m7s.live/v5"
	snap "m7s.live/v5/plugin/snap/pkg"
)

var _ = m7s.InstallPlugin[SnapPlugin](snap.NewTransform)

type SnapPlugin struct {
	//pb.UnimplementedApiServer
	m7s.Plugin
	LogToFile          string
	WatermarkText      string `default:"" desc:"水印文字内容"`
	WatermarkFontPath  string `default:"/System/Library/Fonts/STHeiti Light.ttc" desc:"水印字体文件路径"`
	WatermarkFontColor string `default:"rgba(255,165,0,0.3)" desc:"水印字体颜色，支持rgba格式"`
	WatermarkFontSize  int    `default:"36" desc:"水印字体大小"`
	WatermarkOffsetX   int    `default:"0" desc:"水印位置X"`
	WatermarkOffsetY   int    `default:"0" desc:"水印位置Y"`
}
