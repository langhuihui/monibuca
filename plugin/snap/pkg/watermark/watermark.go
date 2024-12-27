package watermark

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"regexp"
	"time"

	"github.com/disintegration/imaging"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// TextConfig 文字配置结构体
type TextConfig struct {
	Text       string         // 水印文字
	Font       *truetype.Font // 字体
	FontSize   float64        // 字体大小
	Spacing    float64        // 字符间距
	RowSpacing float64        // 行间距
	ColSpacing float64        // 列间距
	Rows       int            // 行数
	Cols       int            // 列数
	DPI        float64        // 分辨率
	Color      color.RGBA     // 文字颜色
	Angle      float64        // 旋转角度
	IsGrid     bool
	OffsetX    int
	OffsetY    int
}

// CalculateTextDimensions 计算文字尺寸的公共函数
func CalculateTextDimensions(config TextConfig, face font.Face) (width, height float64) {
	// 计算文字宽度
	var textWidth float64
	prevC := rune(-1)
	for i, c := range config.Text {
		if prevC >= 0 {
			advance := face.Kern(prevC, c)
			textWidth += float64(advance) / 64
		}
		advance, _ := face.GlyphAdvance(c)
		textWidth += float64(advance) / 64
		if i < len(config.Text)-1 {
			textWidth += config.Spacing
		}
		prevC = c
	}

	// 计算文字高度
	metrics := face.Metrics()
	textHeight := float64(metrics.Height) / 64

	return textWidth, textHeight
}

func DrawWatermark(baseImg image.Image, config TextConfig, isDebug bool) (image.Image, error) {
	switch {
	case config.IsGrid:
		return DrawWatermarkGrid(baseImg, config, isDebug)
	default:
		return DrawWatermarkSingle(baseImg, config, isDebug)
	}
}

// formatTimeString 处理文本中的时间格式化
func formatTimeString(text string) string {
	// 使用正则表达式匹配 $T{format} 格式的字符串
	re := regexp.MustCompile(`\$T\{([^}]+)\}`)
	now := time.Now()

	// 替换所有匹配的时间格式
	return re.ReplaceAllStringFunc(text, func(match string) string {
		format := re.FindStringSubmatch(match)[1]
		return now.Format(format)
	})
}

func DrawWatermarkSingle(baseImg image.Image, config TextConfig, isDebug bool) (image.Image, error) {
	// 处理时间格式化
	formattedText := formatTimeString(config.Text)
	config.Text = formattedText

	bounds := baseImg.Bounds()
	// width, height := bounds.Dx(), bounds.Dy()

	// 设置字体选项
	opts := truetype.Options{
		Size:    config.FontSize,
		DPI:     config.DPI,
		Hinting: font.HintingFull,
	}
	face := truetype.NewFace(config.Font, &opts)
	defer face.Close()

	// 计算文字尺寸
	textWidth, textHeight := CalculateTextDimensions(config, face)

	// 创建一个与文字大小相同的透明图层
	textImg := image.NewRGBA(image.Rect(0, 0, int(math.Ceil(textWidth)), int(math.Ceil(textHeight))))

	// 设置字体上下文
	c := freetype.NewContext()
	c.SetDPI(config.DPI)
	c.SetFont(config.Font)
	c.SetFontSize(config.FontSize)
	c.SetClip(textImg.Bounds())
	c.SetDst(textImg)
	c.SetSrc(image.NewUniform(config.Color))
	c.SetHinting(font.HintingFull)

	// 绘制文字，从左上角开始
	pt := freetype.Pt(0, int(textHeight))

	prevC := rune(-1)
	for i, char := range config.Text {
		if prevC >= 0 {
			kern := face.Kern(prevC, char)
			pt.X += kern
		}

		_, err := c.DrawString(string(char), pt)
		if err != nil {
			return nil, err
		}

		advance, _ := face.GlyphAdvance(char)
		pt.X += advance
		if i < len(config.Text)-1 {
			pt.X += fixed.Int26_6(config.Spacing * 64)
		}

		prevC = char
	}

	if isDebug {
		imaging.Save(textImg, "watermark.png")
	}

	// 创建最终图像
	finalImg := image.NewRGBA(bounds)

	// 复制原图
	draw.Draw(finalImg, bounds, baseImg, image.Point{}, draw.Src)

	// 确保偏移量在图像范围内
	x := config.OffsetX
	y := config.OffsetY

	// 如果需要旋转
	if config.Angle != 0 {
		// 旋转文字，以文字左上角为圆心
		rotated := imaging.Rotate(textImg, config.Angle, color.Transparent)
		if isDebug {
			imaging.Save(rotated, "rotated_watermark.png")
		}

		rotatedBounds := rotated.Bounds()

		// 归化角度到-180到180之间
		normalizedAngle := normalizeAngle(config.Angle)

		// 根据角度范围决定对齐方式
		var rect image.Rectangle
		switch {
		case normalizedAngle > 0 && normalizedAngle <= 90:
			// 左下角对齐
			rect = image.Rect(x, y-rotatedBounds.Dy(), x+rotatedBounds.Dx(), y)
		case normalizedAngle > 90 && normalizedAngle <= 180:
			// 右下角对齐
			rect = image.Rect(x-rotatedBounds.Dx(), y-rotatedBounds.Dy(), x, y)
		case normalizedAngle > -90 && normalizedAngle <= 0:
			// 左上角对齐
			rect = image.Rect(x, y, x+rotatedBounds.Dx(), y+rotatedBounds.Dy())
		default: // -180 到 -90
			// 右上角对齐
			rect = image.Rect(x-rotatedBounds.Dx(), y, x, y+rotatedBounds.Dy())
		}

		// 混合旋转后的文字图层
		draw.Draw(finalImg,
			rect,
			rotated,
			rotatedBounds.Min,
			draw.Over)
	} else {
		// 不旋转时直接绘制
		draw.Draw(finalImg,
			image.Rect(x, y, x+textImg.Bounds().Dx(), y+textImg.Bounds().Dy()),
			textImg,
			textImg.Bounds().Min,
			draw.Over)
	}

	return finalImg, nil
}

// normalizeAngle 将角度归化到-180到180之间
func normalizeAngle(angle float64) float64 {
	// 先归化到0-360
	angle = math.Mod(angle, 360)
	if angle > 180 {
		angle -= 360
	} else if angle <= -180 {
		angle += 360
	}
	return angle
}

// DrawWatermark 绘制水印网格并旋转
func DrawWatermarkGrid(baseImg image.Image, config TextConfig, isDebug bool) (image.Image, error) {
	// 处理时间格式化
	formattedText := formatTimeString(config.Text)
	config.Text = formattedText

	bounds := baseImg.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	opts := truetype.Options{
		Size:    config.FontSize,
		DPI:     config.DPI,
		Hinting: font.HintingFull,
	}
	face := truetype.NewFace(config.Font, &opts)
	defer face.Close()

	// 计算矩形的外接圆半径
	radius := math.Sqrt(float64(width*width+height*height)) / 2

	// 计算外接圆的外切正方形边长
	squareSize := int(math.Ceil(radius * 2))

	// 如果行数或列数为0，自动计算
	if config.Rows == 0 || config.Cols == 0 {
		textWidth, textHeight := CalculateTextDimensions(config, face)

		// 计算单个水印占用的空间
		cellWidth := textWidth + config.ColSpacing
		cellHeight := textHeight + config.RowSpacing

		// 自动计算行数和列数
		if config.Cols == 0 {
			config.Cols = int(math.Floor(float64(squareSize) / cellWidth))
		}
		if config.Rows == 0 {
			config.Rows = int(math.Floor(float64(squareSize) / cellHeight))
		}

		// 确保至少有一行一列
		if config.Cols < 1 {
			config.Cols = 1
		}
		if config.Rows < 1 {
			config.Rows = 1
		}
	}

	// 创建一个正方形的透明图层
	textImg := image.NewRGBA(image.Rect(0, 0, squareSize, squareSize))

	// 设置字体上下文
	c := freetype.NewContext()
	c.SetDPI(config.DPI)
	c.SetFont(config.Font)
	c.SetFontSize(config.FontSize)
	c.SetClip(textImg.Bounds())
	c.SetDst(textImg)
	c.SetSrc(image.NewUniform(config.Color))
	c.SetHinting(font.HintingFull)

	textWidth, textHeight := CalculateTextDimensions(config, face)

	// 计算网格的总宽度和高度
	gridWidth := textWidth + config.ColSpacing
	gridHeight := textHeight + config.RowSpacing

	// 计算起始位置，使整个网格居中
	startX := (float64(squareSize) - (float64(config.Cols)*gridWidth - config.ColSpacing)) / 2
	startY := (float64(squareSize) - (float64(config.Rows)*gridHeight - config.RowSpacing)) / 2

	// 绘制文字网格
	for row := 0; row < config.Rows; row++ {
		for col := 0; col < config.Cols; col++ {
			x := startX + float64(col)*gridWidth
			y := startY + float64(row)*gridHeight

			pt := freetype.Pt(
				int(x),
				int(y+textHeight),
			)

			prevC := rune(-1)
			for i, char := range config.Text {
				if prevC >= 0 {
					kern := face.Kern(prevC, char)
					pt.X += kern
				}

				_, err := c.DrawString(string(char), pt)
				if err != nil {
					return nil, err
				}

				advance, _ := face.GlyphAdvance(char)
				pt.X += advance
				if i < len(config.Text)-1 {
					pt.X += fixed.Int26_6(config.Spacing * 64)
				}

				prevC = char
			}
		}
	}

	if isDebug {
		// 保存文字模板
		imaging.Save(textImg, "watermark.png")
	}
	// 旋转整个文字网格
	rotated := imaging.Rotate(textImg, config.Angle, color.Transparent)
	if isDebug {
		// 保存旋转后的文字模板
		imaging.Save(rotated, "rotated_watermark.png")
	}

	// 创建最终图像
	finalImg := image.NewRGBA(bounds)

	// 复制原图
	draw.Draw(finalImg, bounds, baseImg, image.Point{}, draw.Src)

	// 计算旋转后图片的位置（确保居中）
	rotatedBounds := rotated.Bounds()
	x := (width - rotatedBounds.Dx()) / 2
	y := (height - rotatedBounds.Dy()) / 2
	if isDebug {
		fmt.Printf("width: %d, height: %d, x: %d, y: %d\n", width, height, x, y)
	}
	// 混合旋转后的文字图层
	draw.Draw(finalImg,
		image.Rect(x, y, x+rotatedBounds.Dx(), y+rotatedBounds.Dy()),
		rotated,
		rotatedBounds.Min,
		draw.Over)

	return finalImg, nil
}

// CreateTextColor 创建文字颜色
func CreateTextColor(r, g, b uint8, a uint8) *image.Uniform {
	return image.NewUniform(color.RGBA{r, g, b, a})
}
