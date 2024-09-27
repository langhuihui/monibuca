package plugin_transcode

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"m7s.live/m7s/v5/pkg/config"
	transcode "m7s.live/m7s/v5/plugin/transcode/pkg"
)

type OverlayConfig struct {
	OverlayStream   string `json:"overlay_stream"`   // 叠加流 可为空
	OverlayRegion   string `json:"region"`           //x,y,w,h 可为空,所有区域
	OverlayImage    string `json:"image"`            // 图片 base64  可为空 如果图片和视频流都有，则使用图片
	OverlayPosition string `json:"overlay_position"` //位置 x,y
	Text            string `json:"text"`             // 文字
	FontName        string `json:"font_name"`        //字体文件名
	FontSize        int    `json:"font_size"`
	FontColor       string `json:"font_color"`    // r,g,b 颜色
	TextPosition    string `json:"text_position"` //x,y 文字在图片上的位置
	imagePath       string `json:"-"`
}

type OnDemandTrans struct {
	SrcStream      string           `json:"src_stream"` //原始流
	DstStream      string           `json:"dst_stream"` //输出流
	OverlayConfigs []*OverlayConfig `json:"overlay_config"`
	Encodec        string           `json:"encodec"`
	Decodec        string           `json:"decodec"`
	Scale          string           `json:"scale"`
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

func parseCoordinates(coordString string) (string, error) {

	if coordString == "" {
		return "", nil
	}
	coords := strings.Split(coordString, ",")

	if len(coords) != 2 {
		return "", fmt.Errorf("坐标格式不正确，应该是 x,y")
	}
	x := strings.TrimSpace(coords[0])
	y := strings.TrimSpace(coords[1])
	return fmt.Sprintf("x=%s:y=%s", x, y), nil
}
func parseCrop(cropString string) (string, error) {
	if cropString == "" {
		return "", nil
	}
	cropValues := strings.Split(cropString, ",")
	if len(cropValues) != 4 {
		return "", fmt.Errorf("裁剪参数格式不正确，应该是 x,y,w,h")
	}
	x := strings.TrimSpace(cropValues[0])
	y := strings.TrimSpace(cropValues[1])
	w := strings.TrimSpace(cropValues[2])
	h := strings.TrimSpace(cropValues[3])
	return fmt.Sprintf("crop=%s:%s:%s:%s", x, y, w, h), nil
}

func (t *TranscodePlugin) api_transcode_start(w http.ResponseWriter, r *http.Request) {
	//解析出 OverlayConfigs
	var transReq OnDemandTrans
	err := json.NewDecoder(r.Body).Decode(&transReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var inputs []string
	var filters string
	//循环判断
	for _, overlayConfig := range transReq.OverlayConfigs {
		if overlayConfig.OverlayImage == "" && overlayConfig.Text == "" && overlayConfig.OverlayStream == "" {
			http.Error(w, "image_base64 and text is required", http.StatusBadRequest)
			return
		}
		filePath, err := createTmpImage(overlayConfig.OverlayImage)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		overlayConfig.imagePath = filePath

		// 将 r,g,b 颜色字符串转换为十六进制颜色
		overlayConfig.FontColor, err = rgbToHex(overlayConfig.FontColor)
		if err != nil {
			http.Error(w, "FontColor 格式不正确", http.StatusBadRequest)
			return
		}

		//坐标
		overlayConfig.OverlayPosition, err = parseCoordinates(overlayConfig.OverlayPosition)
		if err != nil {
			http.Error(w, "OverlayPosition 格式不正确", http.StatusBadRequest)
			return
		}
		overlayConfig.OverlayRegion, err = parseCrop(overlayConfig.OverlayRegion)
		if err != nil {
			http.Error(w, "OverlayRegion 格式不正确", http.StatusBadRequest)
			return
		}
		overlayConfig.TextPosition, err = parseCoordinates(overlayConfig.TextPosition)
		if err != nil {
			http.Error(w, "TextPosition 格式不正确", http.StatusBadRequest)
			return
		}
		if overlayConfig.imagePath != "" {
			inputs = append(inputs, overlayConfig.imagePath)
		} else if overlayConfig.OverlayStream != "" {
			inputs = append(inputs, overlayConfig.OverlayStream)
		}
		filters += ""

	}

	//ffmpeg -i input_video.mp4 -i input_image.png -filter_complex "[1:v]drawtext=fontfile=/path/to/font.ttf:fontsize=24:fontcolor=white:x=10:y=10:text='Your Text Here'[img];[0:v][img]overlay=x=100:y=100:enable='between(t,0,5)'" output_video.mp4

	// trans := transcode.NewTransform()
	// trans.(*transcode.Transformer).Start()

	//把 overlayconfig 转为 filter_complex

	transformer := t.Meta.Transformer()

	transcode := transformer.(*transcode.Transformer)
	var cfg config.Transform

	// 解析URL路径
	targetURL := transReq.DstStream
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "无效的目标URL", http.StatusBadRequest)
		return
	}

	// 获取路径部分并清理
	streamPath := path.Clean(parsedURL.Path)
	// 去掉开头的斜杠
	streamPath = strings.TrimPrefix(streamPath, "/")

	conf := strings.Join(inputs, " ") + filters + transReq.Scale + transReq.Encodec
	cfg.Output = []config.TransfromOutput{
		{
			Target:     targetURL,
			StreamPath: streamPath,
			Conf:       conf,
		},
	}

	t.Transform(transReq.SrcStream, cfg)
	fmt.Println(transcode, cfg)
	w.Write([]byte("ok"))
}
