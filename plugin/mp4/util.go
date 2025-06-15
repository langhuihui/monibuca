package plugin_mp4

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"os"
	"os/exec"

	mp4 "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

func saveAsJPG(img image.Image, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	opt := jpeg.Options{Quality: 90}
	return jpeg.Encode(file, img, &opt)
}

func ExtractH264SPSPPS(extraData []byte) (sps, pps []byte, err error) {
	if len(extraData) < 7 {
		return nil, nil, fmt.Errorf("extradata too short")
	}

	// 解析 SPS 数量 (第6字节低5位)
	spsCount := int(extraData[5] & 0x1F)
	offset := 6 // 当前解析位置

	// 提取 SPS
	for i := 0; i < spsCount; i++ {
		if offset+2 > len(extraData) {
			return nil, nil, fmt.Errorf("invalid sps length")
		}
		spsLen := int(binary.BigEndian.Uint16(extraData[offset : offset+2]))
		offset += 2
		if offset+spsLen > len(extraData) {
			return nil, nil, fmt.Errorf("sps data overflow")
		}
		sps = extraData[offset : offset+spsLen]
		offset += spsLen
	}

	// 提取 PPS 数量
	if offset >= len(extraData) {
		return nil, nil, fmt.Errorf("missing pps count")
	}
	ppsCount := int(extraData[offset])
	offset++

	// 提取 PPS
	for i := 0; i < ppsCount; i++ {
		if offset+2 > len(extraData) {
			return nil, nil, fmt.Errorf("invalid pps length")
		}
		ppsLen := int(binary.BigEndian.Uint16(extraData[offset : offset+2]))
		offset += 2
		if offset+ppsLen > len(extraData) {
			return nil, nil, fmt.Errorf("pps data overflow")
		}
		pps = extraData[offset : offset+ppsLen]
		offset += ppsLen
	}
	return sps, pps, nil
}

// 转换函数（支持动态插入参数集）
func ConvertAVCCH264ToAnnexB(data []byte, extraData []byte, isFirst *bool) ([]byte, error) {
	var buf bytes.Buffer
	pos := 0

	for pos < len(data) {
		if pos+4 > len(data) {
			break
		}
		nalSize := binary.BigEndian.Uint32(data[pos : pos+4])
		pos += 4
		nalStart := pos
		pos += int(nalSize)
		if pos > len(data) {
			break
		}
		nalu := data[nalStart:pos]
		nalType := nalu[0] & 0x1F

		// 关键帧前插入SPS/PPS（仅需执行一次）
		if *isFirst && nalType == 5 {
			sps, pps, err := ExtractH264SPSPPS(extraData)
			if err != nil {
				//panic(err)
				return nil, err
			}
			buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
			buf.Write(sps)
			buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
			buf.Write(pps)
			//buf.Write(videoTrack.ExtraData)
			*isFirst = false // 仅首帧插入
		}

		// 保留SEI单元（类型6）和所有其他单元
		if nalType == 5 || nalType == 6 { // IDR/SEI用4字节起始码
			buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
		} else {
			buf.Write([]byte{0x00, 0x00, 0x01}) // 其他用3字节
		}
		buf.Write(nalu)
	}
	return buf.Bytes(), nil
}

/*
H.264与H.265的AVCC格式差异​​
VPS引入​​：H.265新增视频参数集（VPS），用于描述多层编码、时序等信息
*/
// 提取H.265的VPS/SPS/PPS（HEVCDecoderConfigurationRecord格式）
func ExtractHEVCParams(extraData []byte) (vps, sps, pps []byte, err error) {
	if len(extraData) < 22 {
		return nil, nil, nil, errors.New("extra data too short")
	}

	// HEVC的extradata格式参考ISO/IEC 14496-15
	offset := 22 // 跳过头部22字节
	if offset+2 > len(extraData) {
		return nil, nil, nil, errors.New("invalid extra data")
	}

	numOfArrays := int(extraData[offset])
	offset++

	for i := 0; i < numOfArrays; i++ {
		if offset+3 > len(extraData) {
			break
		}

		naluType := extraData[offset] & 0x3F
		offset++
		count := int(binary.BigEndian.Uint16(extraData[offset:]))
		offset += 2

		for j := 0; j < count; j++ {
			if offset+2 > len(extraData) {
				break
			}

			naluSize := int(binary.BigEndian.Uint16(extraData[offset:]))
			offset += 2

			if offset+naluSize > len(extraData) {
				break
			}

			naluData := extraData[offset : offset+naluSize]
			offset += naluSize

			// 根据类型存储参数集
			switch naluType {
			case 32: // VPS
				if vps == nil {
					vps = make([]byte, len(naluData))
					copy(vps, naluData)
				}
			case 33: // SPS
				if sps == nil {
					sps = make([]byte, len(naluData))
					copy(sps, naluData)
				}
			case 34: // PPS
				if pps == nil {
					pps = make([]byte, len(naluData))
					copy(pps, naluData)
				}
			}
		}
	}

	if vps == nil || sps == nil || pps == nil {
		return nil, nil, nil, errors.New("missing required parameter sets")
	}

	return vps, sps, pps, nil
}

// H.265的AVCC转Annex B
func ConvertAVCCHEVCToAnnexB(data []byte, extraData []byte, isFirst *bool) ([]byte, error) {
	var buf bytes.Buffer
	pos := 0

	// 首帧插入VPS/SPS/PPS
	if *isFirst {
		vps, sps, pps, err := ExtractHEVCParams(extraData)
		if err == nil {
			buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
			buf.Write(vps)
			buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
			buf.Write(sps)
			buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
			buf.Write(pps)
		} else {
			return nil, err
		}
	}

	// 处理NALU
	for pos < len(data) {
		if pos+4 > len(data) {
			break
		}
		nalSize := binary.BigEndian.Uint32(data[pos : pos+4])
		pos += 4
		nalStart := pos
		pos += int(nalSize)
		if pos > len(data) {
			break
		}
		nalu := data[nalStart:pos]
		nalType := (nalu[0] >> 1) & 0x3F // H.265的NALU类型在头部的第2-7位

		// 关键帧或参数集使用4字节起始码
		if nalType == 19 || nalType == 20 || nalType >= 32 && nalType <= 34 {
			buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
		} else {
			buf.Write([]byte{0x00, 0x00, 0x01})
		}
		buf.Write(nalu)
	}
	return buf.Bytes(), nil
}

// ffmpeg -hide_banner -i gop.mp4 -vf "select=eq(n\,15)" -vframes 1 -f image2 -pix_fmt bgr24 output.bmp
func ProcessWithFFmpeg(samples []box.Sample, index int, videoTrack *mp4.Track) (image.Image, error) {
	// code := "h264"
	// if videoTrack.Cid == box.MP4_CODEC_H265 {
	// 	code = "hevc"
	// }
	cmd := exec.Command("ffmpeg",
		"-hide_banner",
		//"-f", code, //"h264"  强制指定输入格式为H.264裸流
		"-i", "pipe:0",
		"-vf", fmt.Sprintf("select=eq(n\\,%d)", index),
		"-vframes", "1",
		"-pix_fmt", "bgr24",
		"-f", "rawvideo",
		"pipe:1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go func() {
		errOutput, _ := io.ReadAll(stderr)
		log.Printf("FFmpeg stderr: %s", errOutput)
	}()

	if err = cmd.Start(); err != nil {
		log.Printf("cmd.Start失败: %v", err)
		return nil, err
	}

	go func() {
		defer stdin.Close()
		isFirst := true
		for _, sample := range samples {

			if videoTrack.Cid == box.MP4_CODEC_H264 {
				annexb, _ := ConvertAVCCH264ToAnnexB(sample.Data, videoTrack.ExtraData, &isFirst)
				if _, err := stdin.Write(annexb); err != nil {
					log.Printf("写入失败: %v", err)
					break
				}
			} else {
				annexb, _ := ConvertAVCCHEVCToAnnexB(sample.Data, videoTrack.ExtraData, &isFirst)
				if _, err := stdin.Write(annexb); err != nil {
					log.Printf("写入失败: %v", err)
					break
				}
			}
		}
	}()

	// 读取原始RGB数据
	var buf bytes.Buffer
	if _, err = io.Copy(&buf, stdout); err != nil {
		log.Printf("读取失败: %v", err)
		return nil, err
	}
	if err = cmd.Wait(); err != nil {
		log.Printf("cmd.Wait失败: %v", err)
		return nil, err
	}

	//log.Printf("ffmpeg 提取成功: data size:%v", buf.Len())

	// 转换为image.Image对象
	data := buf.Bytes()
	//width, height := parseBMPDimensions(data)

	width := int(videoTrack.Width)
	height := int(videoTrack.Height)

	log.Printf("ffmpeg size: %v,%v", width, height)

	//FFmpeg的 rawvideo 输出默认采用​​从上到下​​的扫描方式

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			//pos := (height-y-1)*width*3 + x*3
			pos := (y*width + x) * 3 // 关键修复：按行顺序读取
			img.Set(x, y, color.RGBA{
				R: data[pos+2],
				G: data[pos+1],
				B: data[pos],
				A: 255,
			})
		}
	}
	return img, nil
}
