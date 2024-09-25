package plugin_transcode

import (
	"encoding/base64"
	"fmt"
	"m7s.live/m7s/v5/pkg/config"
	transcode "m7s.live/m7s/v5/plugin/transcode/pkg"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type OverlayConfig struct {
	OverlayStream   string `json:"overlay_stream"`   // 叠加流 可为空
	OverlayRegion   string `json:"region"`           //x,y,w,h 可为空,所有区域
	OverlayImage    string `json:"image"`            // 图片 base64  可为空
	OverlayPosition string `json:"overlay_position"` //位置 x,y
	Text            string `json:"text"`             // 文字
	FontName        string `json:"font_name"`        //字体文件名
	FontSize        int    `json:"font_size"`
	FontColor       string `json:"font_color"`    // r,g,b 颜色
	TextPosition    string `json:"text_position"` //x,y 文字在图片上的位置
	imagePath       string `json:"-"`
}

type OnDemandTrans struct {
	SrcStream      string           `json:"src_stream"`
	DstStream      string           `json:"dst_stream"`
	OverlayConfigs []*OverlayConfig `json:"overlay_config"`
}

func createTmpImage(image string) (string, error) {

	//通过前缀判断base64图片类型
	var imageType string
	switch {
	case strings.HasPrefix(image, "/9j/"):
		imageType = "jpg"
	case strings.HasPrefix(image, "iVBORw0KGg"):
		imageType = "png"
	case strings.HasPrefix(image, "R0lGODlh"):
		imageType = "gif"
	case strings.HasPrefix(image, "UklGRg"):
		imageType = "webp"
	default:
		return "", fmt.Errorf("不支持的图片类型")
	}

	// 创建一个临时文件
	tempFile, err := os.CreateTemp("./logs", "overlay*."+imageType)

	if err != nil {
		return "", fmt.Errorf("创建临时文件失败")
	}
	// 按照文件类型解码 base64 写入文件
	decodedData, err := base64.StdEncoding.DecodeString(image)
	if err != nil {
		return "", fmt.Errorf("解码 base64 失败")
	}
	// 将解码后的数据写入临时文件
	tempFile.Write(decodedData)
	//文件路径
	filePath := tempFile.Name()
	return filePath, nil
}

func rgbToHex(FontColor string) (string, error) {
	rgb := strings.Split(FontColor, ",")
	if len(rgb) == 3 {
		r, _ := strconv.Atoi(rgb[0])
		g, _ := strconv.Atoi(rgb[1])
		b, _ := strconv.Atoi(rgb[2])
		FontColor = fmt.Sprintf("#%02x%02x%02x", r, g, b)
		return FontColor, nil
	} else {
		return "", fmt.Errorf("FontColor 格式不正确")

	}
}

func (t *TranscodePlugin) api_transcode_start(w http.ResponseWriter, r *http.Request) {
	//解析出 OverlayConfigs
	//var trans OnDemandTrans
	//err := json.NewDecoder(r.Body).Decode(&trans)
	//if err != nil {
	//	http.Error(w, err.Error(), http.StatusBadRequest)
	//	return
	//}
	////循环判断
	//for _, overlayConfig := range trans.OverlayConfigs {
	//	if overlayConfig.OverlayImage == "" || overlayConfig.Text == "" {
	//		http.Error(w, "image_base64 and text is required", http.StatusBadRequest)
	//		return
	//	}
	//	filePath, err := createTmpImage(overlayConfig.OverlayImage)
	//	if err != nil {
	//		http.Error(w, err.Error(), http.StatusBadRequest)
	//		return
	//	}
	//	overlayConfig.imagePath = filePath
	//	overlayConfig.FontName = "./font/" + overlayConfig.FontName
	//
	//	// 将 r,g,b 颜色字符串转换为十六进制颜色
	//	overlayConfig.FontColor, err = rgbToHex(overlayConfig.FontColor)
	//	if err != nil {
	//		http.Error(w, "FontColor 格式不正确", http.StatusBadRequest)
	//		return
	//	}
	//}

	// 1、先叠加，不用管参数
	// 2、参数处理

	//ffmpeg -i input_video.mp4 -i input_image.png -filter_complex "[1:v]drawtext=fontfile=/path/to/font.ttf:fontsize=24:fontcolor=white:x=10:y=10:text='Your Text Here'[img];[0:v][img]overlay=x=100:y=100:enable='between(t,0,5)'" output_video.mp4

	//trans := transcode.NewTransform()
	//trans.(*transcode.Transformer).Start()

	transformer := t.Meta.Transformer()

	trans := transformer.(*transcode.Transformer)
	var cfg config.Transform

	cfg.Output = []struct {
		Target     string `desc:"转码目标"`
		StreamPath string
		Conf       any
	}{
		{
			Target:     "rtmp://127.0.0.1:1935/live/test/h264",
			StreamPath: "live/test/h264",
			Conf:       "-loglevel debug -s 480x200 -c:a copy -c:v h264",
		},
	}
	t.Transform("live/test", cfg)
	fmt.Println(trans, cfg)
	w.Write([]byte("ok"))
}
