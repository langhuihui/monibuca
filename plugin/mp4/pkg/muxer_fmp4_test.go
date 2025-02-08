package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

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

// validateAndFixAVCC 验证并修复 AVCC 格式的 NALU
func validateAndFixAVCC(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("data too short for AVCC")
	}

	var pos int
	var output []byte

	for pos < len(data) {
		if pos+4 > len(data) {
			return nil, fmt.Errorf("incomplete NALU length at position %d", pos)
		}

		// 读取 NALU 长度（4字节，大端序）
		naluLen := binary.BigEndian.Uint32(data[pos : pos+4])

		// 验证 NALU 长度
		if naluLen == 0 || pos+4+int(naluLen) > len(data) {
			return nil, fmt.Errorf("invalid NALU length %d at position %d", naluLen, pos)
		}

		// 验证 NALU 类型
		naluType := data[pos+4] & 0x1F
		if naluType == 0 || naluType > 12 {
			return nil, fmt.Errorf("invalid NALU type %d at position %d", naluType, pos)
		}

		// 复制长度前缀和 NALU 数据
		output = append(output, data[pos:pos+4+int(naluLen)]...)
		pos += 4 + int(naluLen)
	}

	return output, nil
}

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
	flvFile, err := os.Open("/Users/dexter/Movies/frame_counter_4k_60fps.flv")
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

	// Skip to the first tag
	if _, err := flvFile.Seek(int64(header.DataOffset), io.SeekStart); err != nil {
		t.Fatalf("Failed to seek to first tag: %v", err)
	}

	// Create tracks
	var videoTrack *Track
	if hasVideo {
		videoTrack = muxer.AddTrack(box.MP4_CODEC_H264)
		videoTrack.Width = 3840 // 4K resolution
		videoTrack.Height = 2160
		videoTrack.Timescale = 1000
	}

	// Variables to store codec configuration
	var videoConfig []byte
	var frameCount int

	// Process FLV tags
	// TagLoop:
	for {
		tag, err := readFLVTag(flvFile)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("Failed to read FLV tag: %v", err)
		}

		switch tag.TagType {
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
					videoTrack.ExtraData = videoConfig
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

					// 验证和修复 AVCC 格式
					validData, err := validateAndFixAVCC(tag.Data[5:])
					if err != nil {
						fmt.Printf("Warning: Invalid AVCC data at timestamp %d: %v\n", tag.Timestamp, err)
						continue
					}

					sample := box.Sample{
						Data:     validData,
						DTS:      uint64(tag.Timestamp),
						PTS:      uint64(int64(tag.Timestamp) + int64(compositionTime)),
						KeyFrame: frameType == 1,
					}
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
