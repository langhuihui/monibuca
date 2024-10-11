package plugin_transcode

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	globalPB "m7s.live/m7s/v5/pb"
	"m7s.live/m7s/v5/plugin/transcode/pb"
	transcode "m7s.live/m7s/v5/plugin/transcode/pkg"

	"m7s.live/m7s/v5/pkg/config"
)

func createTmpImage(image string) (string, error) {
	if image == "" {
		return "", nil
	}
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

func parseFontColor(FontColor string) (string, error) {
	rgb := strings.Split(FontColor, ",")
	rgbLen := len(rgb)
	switch rgbLen {
	case 3:
		r, _ := strconv.Atoi(rgb[0])
		g, _ := strconv.Atoi(rgb[1])
		b, _ := strconv.Atoi(rgb[2])
		FontColor = fmt.Sprintf(":fontcolor=#%02x%02x%02x", r, g, b)
		return FontColor, nil
	case 1:
		if rgb[0] == "" {
			FontColor = ":fontcolor=white"
		} else if strings.HasPrefix(rgb[0], "#") && len(rgb[0]) == 7 {
			FontColor = ":fontcolor=" + rgb[0]
		} else {
			return "", fmt.Errorf("FontColor 格式不正确")
		}
		return FontColor, nil
	default:
		return "", fmt.Errorf("FontColor 格式不正确")
	}
}

// fontfile
func parseFontFile(fontFile string) (string, error) {
	if fontFile == "" {
		return "", nil
	}
	//判断文件是否存在
	if _, err := os.Stat(fontFile); os.IsNotExist(err) {
		return "", fmt.Errorf("fontFile 文件不存在")
	}
	return fmt.Sprintf(":fontfile=%s", fontFile), nil
}

// fontsize
func parseFontSize(fontSize string) (string, error) {
	if fontSize == "" {
		return "", nil
	}
	size, err := strconv.Atoi(fontSize)
	if err != nil {
		return "", fmt.Errorf("fontSize 格式不正确")
	}
	if size < 0 {
		return "", fmt.Errorf("fontSize 不能小于0")
	}
	return fmt.Sprintf(":fontsize=%d", size), nil
}
func parseCoordinates(coordString string) (string, error) {

	if coordString == "" {
		return "x=0:y=0", nil
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
	w := strings.TrimSpace(cropValues[0])
	h := strings.TrimSpace(cropValues[1])
	x := strings.TrimSpace(cropValues[2])
	y := strings.TrimSpace(cropValues[3])
	return fmt.Sprintf("crop=%s:%s:%s:%s", x, y, w, h), nil
}

func (t *TranscodePlugin) Launch(ctx context.Context, transReq *pb.TransRequest) (response *globalPB.SuccessResponse, err error) {

	response = &globalPB.SuccessResponse{}
	defer func() {
		if err != nil {
			response.Code = -1
			response.Message = err.Error()
		} else {
			response.Code = 0
			response.Message = "success"
		}
	}()
	var (
		filters []string
		out     string
		conf    string
		vIdx    int //视频
		tIdx    int //文字
	)

	inputs := []string{""}
	lastOverlay := "[0:v]"
	for _, overlayConfig := range transReq.OverlayConfigs {
		if overlayConfig.OverlayImage == "" && overlayConfig.Text == "" && overlayConfig.OverlayStream == "" {
			err = fmt.Errorf("image_base64 and text is required")
			return
		}
		var filePath string
		filePath, err = createTmpImage(overlayConfig.OverlayImage)
		if err != nil {
			return
		}
		overlayConfig.OverlayImage = filePath

		// 将 r,g,b 颜色字符串转换为十六进制颜色
		overlayConfig.FontColor, err = parseFontColor(overlayConfig.FontColor)
		if err != nil {
			return
		}
		overlayConfig.FontName, err = parseFontFile(overlayConfig.FontName)
		if err != nil {
			return
		}
		// 字体大小
		overlayConfig.FontSize, err = parseFontSize(overlayConfig.FontSize)
		if err != nil {
			return
		}

		//坐标
		overlayConfig.OverlayPosition, err = parseCoordinates(overlayConfig.OverlayPosition)
		if err != nil {
			return
		}
		overlayConfig.OverlayRegion, err = parseCrop(overlayConfig.OverlayRegion)
		if err != nil {
			return
		}
		overlayConfig.TextPosition, err = parseCoordinates(overlayConfig.TextPosition)
		if err != nil {
			return
		}
		overlayConfig.TextPosition = ":" + overlayConfig.TextPosition
		//[1:v]crop=400:300:10:10[overlay];
		if overlayConfig.OverlayImage != "" {
			inputs = append(inputs, overlayConfig.OverlayImage)
		} else if overlayConfig.OverlayStream != "" {
			inputs = append(inputs, overlayConfig.OverlayStream)
		}

		// 生成 filter_complex
		if overlayConfig.OverlayImage != "" || overlayConfig.OverlayStream != "" {
			vIdx++
			if overlayConfig.OverlayRegion != "" {
				filters = append(filters, fmt.Sprintf("[%d:v]%s[overlay%d]", vIdx, overlayConfig.OverlayRegion, vIdx))
			}
			if overlayConfig.OverlayPosition != "" {
				if overlayConfig.OverlayRegion != "" {
					filters = append(filters, fmt.Sprintf("%s[overlay%d]overlay=%s[tmp%d]", lastOverlay, vIdx, overlayConfig.OverlayPosition, vIdx))

				} else {
					filters = append(filters, fmt.Sprintf("%s[%d:v]overlay=%s[tmp%d]", lastOverlay, vIdx, overlayConfig.OverlayPosition, vIdx))
				}
			}
			lastOverlay = fmt.Sprintf("[tmp%d]", vIdx)
			out = lastOverlay
		}
		if overlayConfig.Text != "" {
			tIdx++
			timeText := ""
			if overlayConfig.TimeOffset != 0 {
				//%{pts\\:gmtime\\:1577836800\\:%Y-%m-%d %H\\\\\\:%M\\\\\\:%S}
				timeText = fmt.Sprintf("%%{pts\\:gmtime\\:%d}", overlayConfig.TimeOffset)
			} else {
				timeText = fmt.Sprintf(`%%{localtime}`)
			}
			if overlayConfig.TimeFormat != "" {
				timeText = strings.ReplaceAll(timeText, "}", "\\:"+overlayConfig.TimeFormat+"}")
			}

			if timeText != "" {
				timeText = strings.ReplaceAll(overlayConfig.Text, "$T", timeText)
			}
			if overlayConfig.LineSpacing != "" {
				overlayConfig.LineSpacing = fmt.Sprintf(":line_spacing=%s", overlayConfig.LineSpacing)
			}
			filters = append(filters, fmt.Sprintf("%sdrawtext=text='%s'%s%s%s%s%s[out%d]", lastOverlay, timeText, overlayConfig.FontName, overlayConfig.FontSize, overlayConfig.FontColor, overlayConfig.TextPosition, overlayConfig.LineSpacing, tIdx))
			lastOverlay = fmt.Sprintf("[out%d]", tIdx)
			out = lastOverlay
		}

	}

	var cfg config.Transform

	// 解析URL路径
	targetURL := transReq.DstStream
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		err = fmt.Errorf("无效的目标URL: %s", err)
		return
	}

	// 获取路径部分并清理
	streamPath := path.Clean(parsedURL.Path)
	// 去掉开头的斜杠
	streamPath = strings.TrimPrefix(streamPath, "/")

	// 拼接 ffmpeg 命令
	filterStr := ""
	if len(filters) != 0 {
		filterStr = fmt.Sprintf(" -filter_complex  %s ", strings.Join(filters, ";")) + fmt.Sprintf(" -map %s ", out)
	}

	if transReq.Scale != "" {
		transReq.Scale = fmt.Sprintf(" -s %s ", transReq.Scale)
	}
	if transReq.GlobalOptions != "" {
		transReq.GlobalOptions = fmt.Sprintf(" %s ", transReq.GlobalOptions)
	}
	conf = strings.Join(inputs, " -i ") + fmt.Sprintf(" %s ", filterStr) + transReq.Scale + transReq.Encodec

	cfg.Output = []config.TransfromOutput{
		{
			Target:     targetURL,
			StreamPath: streamPath,
			Conf:       conf,
		},
	}
	cfg.Input = transcode.DecodeConfig{
		Mode:  transcode.TRANS_MODE_RTMP,
		Args:  transReq.GlobalOptions,
		Codec: transReq.Decodec,
	}

	t.Transform(transReq.SrcStream, cfg)
	return
}

func (t *TranscodePlugin) Close(ctx context.Context, closeReq *pb.CloseRequest) (response *globalPB.SuccessResponse, err error) {
	response = &globalPB.SuccessResponse{}
	if item, ok := t.Server.Transforms.Get(closeReq.DstStream); ok {
		item.TransformJob.Stop(fmt.Errorf("manual closed"))
	}
	return
}
