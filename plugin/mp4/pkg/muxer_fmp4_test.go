package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/deepch/vdk/codec/h264parser"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

type (
	FLVHeader struct {
		Signature  [3]byte
		Version    uint8
		Flags      uint8
		DataOffset uint32
	}

	FLVTag struct {
		TagType   uint8
		DataSize  uint32
		Timestamp uint32
		StreamID  uint32
		Data      []byte
	}
)

func readFLVHeader(r io.Reader) (*FLVHeader, error) {
	header := &FLVHeader{}
	if err := binary.Read(r, binary.BigEndian, &header.Signature); err != nil {
		return nil, fmt.Errorf("error reading signature: %v", err)
	}
	if err := binary.Read(r, binary.BigEndian, &header.Version); err != nil {
		return nil, fmt.Errorf("error reading version: %v", err)
	}
	if err := binary.Read(r, binary.BigEndian, &header.Flags); err != nil {
		return nil, fmt.Errorf("error reading flags: %v", err)
	}
	if err := binary.Read(r, binary.BigEndian, &header.DataOffset); err != nil {
		return nil, fmt.Errorf("error reading data offset: %v", err)
	}

	// Validate FLV signature
	if string(header.Signature[:]) != "FLV" {
		return nil, fmt.Errorf("invalid FLV signature: %s", string(header.Signature[:]))
	}

	fmt.Printf("FLV Header: Version=%d, Flags=%d, DataOffset=%d\n", header.Version, header.Flags, header.DataOffset)
	return header, nil
}

func readFLVTag(r io.Reader) (*FLVTag, error) {
	tag := &FLVTag{}

	// Read previous tag size (4 bytes)
	var prevTagSize uint32
	if err := binary.Read(r, binary.BigEndian, &prevTagSize); err != nil {
		return nil, err
	}

	// Read tag type (1 byte)
	if err := binary.Read(r, binary.BigEndian, &tag.TagType); err != nil {
		return nil, err
	}

	// Read data size (3 bytes)
	var dataSize [3]byte
	if _, err := io.ReadFull(r, dataSize[:]); err != nil {
		return nil, err
	}
	tag.DataSize = uint32(dataSize[0])<<16 | uint32(dataSize[1])<<8 | uint32(dataSize[2])

	// Read timestamp (3 bytes + 1 byte extended)
	var timestamp [3]byte
	if _, err := io.ReadFull(r, timestamp[:]); err != nil {
		return nil, err
	}
	var timestampExtended uint8
	if err := binary.Read(r, binary.BigEndian, &timestampExtended); err != nil {
		return nil, err
	}
	tag.Timestamp = uint32(timestamp[0])<<16 | uint32(timestamp[1])<<8 | uint32(timestamp[2]) | uint32(timestampExtended)<<24

	// Read stream ID (3 bytes)
	var streamID [3]byte
	if _, err := io.ReadFull(r, streamID[:]); err != nil {
		return nil, err
	}
	tag.StreamID = uint32(streamID[0])<<16 | uint32(streamID[1])<<8 | uint32(streamID[2])

	// Read tag data
	tag.Data = make([]byte, tag.DataSize)
	if _, err := io.ReadFull(r, tag.Data); err != nil {
		return nil, err
	}

	return tag, nil
}

func findBoxOffsets(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	type boxInfo struct {
		name   string
		offset int64
		size   uint32
	}

	var boxes []boxInfo

	// Read the entire file
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Search for boxes
	var i int
	for i < len(data)-8 {
		// Read box size (4 bytes) and type (4 bytes)
		size := binary.BigEndian.Uint32(data[i : i+4])
		boxType := string(data[i+4 : i+8])

		// Validate box size
		if size < 8 || int64(size) > int64(len(data))-int64(i) {
			i++
			continue
		}

		if boxType == "tfdt" || boxType == "mdat" || boxType == "moof" || boxType == "traf" {
			boxes = append(boxes, boxInfo{
				name:   boxType,
				offset: int64(i),
				size:   size,
			})
			// Print the entire box content for small boxes
			if size <= 256 {
				fmt.Printf("\nFull %s box at offset %d (0x%x):\n", boxType, i, i)
				// Print box header
				fmt.Printf("Header: % x\n", data[i:i+8])
				// Print content in chunks of 32 bytes
				for j := i + 8; j < i+int(size); j += 32 {
					end := j + 32
					if end > i+int(size) {
						end = i + int(size)
					}
					fmt.Printf("Content [%d-%d]: % x\n", j-i, end-i, data[j:end])
				}
			}
		}
		// Move to the next box
		i += int(size)
	}

	// Print box information in order of appearance
	fmt.Println("\nBox layout:")
	for _, box := range boxes {
		fmt.Printf("%s box at offset %d (0x%x), size: %d bytes\n",
			box.name, box.offset, box.offset, box.size)

		// Print the first few bytes of the box content
		start := box.offset + 8 // skip size and type
		end := start + 32
		if end > box.offset+int64(box.size) {
			end = box.offset + int64(box.size)
		}
		fmt.Printf("%s content: % x\n", box.name, data[start:end])

		// For tfdt box, also print the previous and next 8 bytes
		if box.name == "tfdt" {
			prevStart := box.offset - 8
			if prevStart < 0 {
				prevStart = 0
			}
			nextEnd := box.offset + int64(box.size) + 8
			if nextEnd > int64(len(data)) {
				nextEnd = int64(len(data))
			}
			fmt.Printf("Context around tfdt:\n")
			fmt.Printf("Previous 8 bytes: % x\n", data[prevStart:box.offset])
			fmt.Printf("Next 8 bytes: % x\n", data[box.offset+int64(box.size):nextEnd])
		}
	}
	return nil
}

func TestFLVToFMP4(t *testing.T) {
	// Open FLV file
	flvFile, err := os.Open("/Users/dexter/Movies/002.flv")
	if err != nil {
		t.Fatalf("Failed to open FLV file: %v", err)
	}
	defer flvFile.Close()

	// Create output FMP4 file
	outFile, err := os.Create("test.mp4")
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer outFile.Close()

	// Create FMP4 muxer
	muxer := NewMuxer(FLAG_FRAGMENT)
	muxer.WriteInitSegment(outFile)
	// Read FLV header
	header, err := readFLVHeader(flvFile)
	if err != nil {
		t.Fatalf("Failed to read FLV header: %v", err)
	}

	hasVideo := header.Flags&0x01 != 0
	hasAudio := header.Flags&0x04 != 0
	// hasAudio := false
	// Skip to the first tag
	if _, err := flvFile.Seek(int64(header.DataOffset), io.SeekStart); err != nil {
		t.Fatalf("Failed to seek to first tag: %v", err)
	}

	// Create tracks
	var videoTrack, audioTrack *Track
	if hasVideo {
		videoTrack = muxer.AddTrack(box.MP4_CODEC_H264)
		videoTrack.Timescale = 1000
	}
	if hasAudio {
		audioTrack = muxer.AddTrack(box.MP4_CODEC_AAC)
		audioTrack.Timescale = 1000
	}

	// Variables to store codec configuration
	var videoConfig []byte
	var audioConfig []byte
	var frameCount int

	// Process FLV tags
	// TagLoop:
	for {
		tag, err := readFLVTag(flvFile)
		if err != nil {
			// if err == io.EOF {
			// 	break
			// }
			// t.Fatalf("Failed to read FLV tag: %v", err)
			break
		}

		switch tag.TagType {
		case 8: // Audio
			if !hasAudio || audioTrack == nil {
				continue
			}

			soundFormat := tag.Data[0] >> 4
			if soundFormat != 10 { // AAC
				continue
			}

			aacPacketType := tag.Data[1]
			if aacPacketType == 0 { // AAC sequence header
				fmt.Println("Found AAC sequence header")
				// 解析 AAC 配置
				if len(tag.Data) < 4 {
					t.Fatalf("Invalid AAC sequence header")
				}
				audioConfig = tag.Data[2:]
				// 解析音频轨道参数
				audioObjectType := (audioConfig[0] >> 3) & 0x1F
				samplingFrequencyIndex := ((audioConfig[0] & 0x07) << 1) | (audioConfig[1] >> 7)
				channelConfig := (audioConfig[1] >> 3) & 0x0F

				// 设置采样率（根据 ISO/IEC 14496-3 标准）
				sampleRates := []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
				var sampleRate uint32 = 44100 // 默认值
				if int(samplingFrequencyIndex) < len(sampleRates) {
					sampleRate = uint32(sampleRates[samplingFrequencyIndex])
				}

				fmt.Printf("AAC Config: ObjectType=%d, SampleRate=%d, Channels=%d\n",
					audioObjectType, sampleRate, channelConfig)

				// 这里应该创建 AAC codec context，但为了简化测试，我们暂时跳过
				// TODO: 创建适当的 AAC codec context
			} else if len(audioConfig) > 0 { // Audio data
				if len(tag.Data) <= 2 {
					fmt.Printf("Skipping empty audio sample at timestamp %d\n", tag.Timestamp)
					continue
				}

				// 确保只写入原始 AAC 负载（去掉 ADTS 头）
				aacData := tag.Data[2:]
				if len(aacData) < 1 {
					continue
				}

				sample := box.Sample{
					Timestamp: uint32(tag.Timestamp),
					CTS:       0,
					KeyFrame:  true,
				}
				sample.PushOne(aacData)
				if err := muxer.WriteSample(outFile, audioTrack, sample); err != nil {
					t.Fatalf("Failed to write audio sample: %v", err)
				}
				frameCount++
			}
		case 9: // Video
			if !hasVideo || videoTrack == nil {
				continue
			}

			codecID := tag.Data[0] & 0x0f
			frameType := tag.Data[0] >> 4
			if codecID == 7 { // AVC/H.264
				if tag.Data[1] == 0 { // AVC sequence header
					fmt.Println("Found AVC sequence header")
					videoConfig = tag.Data[5:] // Store AVC config (skip composition time)
					codecData, err := h264parser.NewCodecDataFromAVCDecoderConfRecord(videoConfig)
					if err != nil {
						t.Fatalf("Failed to parse AVC sequence header: %v", err)
					}
					fmt.Printf("Codec data: %+v\n", codecData)
					fmt.Printf("Video resolution: %dx%d\n", codecData.Width(), codecData.Height())
					videoTrack.Timescale = 1000

					// 这里应该创建 H264 codec context，但为了简化测试，我们暂时跳过
					// TODO: 创建适当的 H264 codec context

				} else if len(videoConfig) > 0 { // Video data
					if len(tag.Data) <= 5 {
						fmt.Printf("Skipping empty video sample at timestamp %d\n", tag.Timestamp)
						continue
					}

					// Read composition time offset (24 bits, signed)
					compositionTime := int32(tag.Data[2])<<16 | int32(tag.Data[3])<<8 | int32(tag.Data[4])
					// Convert 24-bit signed integer to 32-bit signed integer
					if compositionTime&0x800000 != 0 {
						compositionTime |= ^0xffffff
					}

					sample := box.Sample{
						Timestamp: uint32(tag.Timestamp),
						CTS:       uint32(compositionTime),
						KeyFrame:  frameType == 1,
					}
					sample.PushOne(tag.Data[5:])
					if err := muxer.WriteSample(outFile, videoTrack, sample); err != nil {
						t.Fatalf("Failed to write video sample: %v", err)
					}
					frameCount++

					// if frameCount >= 5 {
					// 	fmt.Println("Wrote 5 frames, stopping")
					// 	break TagLoop
					// }
				}
			}
		}
	}

	// Write trailer
	if err := muxer.WriteTrailer(outFile); err != nil {
		t.Fatalf("Failed to write trailer: %v", err)
	}

	fmt.Println("Conversion completed successfully")

	// Find and analyze box positions
	if err := findBoxOffsets("test.mp4"); err != nil {
		t.Fatalf("Failed to analyze boxes: %v", err)
	}

	// Validate the generated MP4 file using MP4Box
	cmd := exec.Command("MP4Box", "-info", "test.mp4")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("MP4Box validation failed: %v\nOutput: %s", err, output)
	}
	fmt.Printf("MP4Box validation output:\n%s\n", output)

	t.Log("Test completed successfully")
}
