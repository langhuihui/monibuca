package mp4

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

	"m7s.live/v5/plugin/mp4/pkg/box"
)

func TestFLVToMP4(t *testing.T) {
	// Open FLV file
	flvFile, err := os.Open("/Users/dexter/Movies/frame_counter_4k_60fps.flv")
	if err != nil {
		t.Fatalf("Failed to open FLV file: %v", err)
	}
	defer flvFile.Close()

	// Create output MP4 file
	outFile, err := os.Create("test_regular.mp4")
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer outFile.Close()

	// Create regular MP4 muxer (without fragmentation flag)
	muxer := NewMuxer(0) // No flags for regular MP4
	muxer.WriteInitSegment(outFile)

	// Read FLV header
	header, err := readFLVHeader(flvFile)
	if err != nil {
		t.Fatalf("Failed to read FLV header: %v", err)
	}

	hasVideo := header.Flags&0x01 != 0
	hasAudio := header.Flags&0x04 != 0

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
	var videoConfig, audioConfig []byte
	var frameCount, sampleCount int

	// Process FLV tags
	for {
		tag, err := readFLVTag(flvFile)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("Failed to read FLV tag: %v", err)
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
				audioConfig = tag.Data[2:] // Store AAC config
				// 这里应该创建 AAC codec context，但为了简化测试，我们暂时跳过
				// TODO: 创建适当的 AAC codec context
			} else if len(audioConfig) > 0 { // Audio data
				if len(tag.Data) <= 2 {
					fmt.Printf("Skipping empty audio sample at timestamp %d\n", tag.Timestamp)
					continue
				}

				sample := box.Sample{
					Timestamp: uint32(tag.Timestamp),
					CTS:       0,
					KeyFrame:  true, // Audio samples are always key frames
				}
				sample.PushOne(tag.Data[2:])
				if err := muxer.WriteSample(outFile, audioTrack, sample); err != nil {
					t.Fatalf("Failed to write audio sample: %v", err)
				}
				sampleCount++
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
					// 这里应该创建 H264 codec context，但为了简化测试，我们暂时跳过
					// TODO: 创建适当的 H264 codec context
				} else if len(videoConfig) > 0 { // Video data
					if len(tag.Data) <= 5 {
						fmt.Printf("Skipping empty video sample at timestamp %d\n", tag.Timestamp)
						continue
					}

					// Read composition time offset (24 bits, signed)
					compositionTime := int32(tag.Data[2])<<16 | int32(tag.Data[3])<<8 | int32(tag.Data[4])
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
				}
			}
		}
	}

	// Write trailer
	if err := muxer.WriteTrailer(outFile); err != nil {
		t.Fatalf("Failed to write trailer: %v", err)
	}

	fmt.Printf("Conversion completed successfully with %d video frames and %d audio samples\n", frameCount, sampleCount)

	// Validate the generated MP4 file using MP4Box
	cmd := exec.Command("MP4Box", "-info", "test_regular.mp4")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("MP4Box validation failed: %v\nOutput: %s", err, output)
	}
	fmt.Printf("MP4Box validation output:\n%s\n", output)

	t.Log("Test completed successfully")
}
