package snap

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"os"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/golang/freetype/truetype"
	"m7s.live/v5/plugin/snap/pkg/watermark"
)

var (
	fontCache             = make(map[string]*truetype.Font)
	fontCacheLock         sync.RWMutex
	GlobalWatermarkConfig WatermarkConfig
)

// WatermarkConfig 水印配置
type WatermarkConfig struct {
	Text        string         // 水印文字
	FontPath    string         // 字体文件路径
	FontSize    float64        // 字体大小
	FontColor   color.RGBA     // 字体颜色
	FontSpacing float64        // 字体间距
	OffsetX     int            // X轴偏移
	OffsetY     int            // Y轴偏移
	Font        *truetype.Font // 缓存的字体对象
}

// LoadFont 加载字体文件
func (w *WatermarkConfig) LoadFont() error {
	if w.Font != nil {
		return nil
	}

	fontCacheLock.RLock()
	cachedFont, exists := fontCache[w.FontPath]
	fontCacheLock.RUnlock()

	if exists {
		w.Font = cachedFont
		return nil
	}

	// 读取字体文件
	fontBytes, err := os.ReadFile(w.FontPath)
	if err != nil {
		return fmt.Errorf("read font file error: %w", err)
	}

	// 解析字体
	font, err := truetype.Parse(fontBytes)
	if err != nil {
		return fmt.Errorf("parse font error: %w", err)
	}

	fontCacheLock.Lock()
	fontCache[w.FontPath] = font
	fontCacheLock.Unlock()

	w.Font = font
	return nil
}

// AddWatermark 为图片添加水印
func AddWatermark(imgData []byte, config WatermarkConfig) ([]byte, error) {
	if config.Text == "" {
		return imgData, nil
	}

	// 加载字体
	if err := config.LoadFont(); err != nil {
		return nil, err
	}

	// 解码图片
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("decode image error: %w", err)
	}

	// 确保alpha通道正确
	if config.FontColor.A == 0 {
		config.FontColor.A = 255 // 如果完全透明，改为不透明
	}

	// 添加水印
	result, err := watermark.DrawWatermarkSingle(img, watermark.TextConfig{
		Text:       config.Text,
		Font:       config.Font,
		FontSize:   config.FontSize,
		Spacing:    10,
		RowSpacing: 10,
		ColSpacing: 20,
		Rows:       1,
		Cols:       1,
		DPI:        72,
		Color:      config.FontColor,
		IsGrid:     false,
		Angle:      0,
		OffsetX:    config.OffsetX,
		OffsetY:    config.OffsetY,
	}, false)
	if err != nil {
		return nil, fmt.Errorf("add watermark error: %w", err)
	}

	// 编码图片
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, result, imaging.JPEG); err != nil {
		return nil, fmt.Errorf("encode image error: %w", err)
	}

	return buf.Bytes(), nil
}
