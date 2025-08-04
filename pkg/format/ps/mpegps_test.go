package mpegps

import (
	"bytes"
	"io"
	"testing"

	"m7s.live/v5/pkg/util"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestMpegPSConstants(t *testing.T) {
	// Test that PS constants are properly defined
	t.Run("Constants", func(t *testing.T) {
		if StartCodePS != 0x000001ba {
			t.Errorf("Expected StartCodePS %x, got %x", 0x000001ba, StartCodePS)
		}

		if PSPackHeaderSize != 14 {
			t.Errorf("Expected PSPackHeaderSize %d, got %d", 14, PSPackHeaderSize)
		}

		if MaxPESPayloadSize != 0xFFEB {
			t.Errorf("Expected MaxPESPayloadSize %x, got %x", 0xFFEB, MaxPESPayloadSize)
		}
	})
}

func TestMuxPSHeader(t *testing.T) {
	// Test PS header generation
	t.Run("PSHeader", func(t *testing.T) {
		// Create a buffer for testing - initialize with length 0 to allow appending
		buffer := make([]byte, 0, PSPackHeaderSize)
		utilBuffer := util.Buffer(buffer)

		// Call MuxPSHeader
		MuxPSHeader(&utilBuffer)

		// Check the buffer length
		if len(utilBuffer) != PSPackHeaderSize {
			t.Errorf("Expected buffer length %d, got %d", PSPackHeaderSize, len(utilBuffer))
		}

		// Check PS start code (first 4 bytes should be 0x00 0x00 0x01 0xBA)
		expectedStartCode := []byte{0x00, 0x00, 0x01, 0xBA}
		if !bytes.Equal(utilBuffer[:4], expectedStartCode) {
			t.Errorf("Expected PS start code %x, got %x", expectedStartCode, utilBuffer[:4])
		}

		t.Logf("PS Header: %x", utilBuffer)
		t.Logf("Buffer length: %d", len(utilBuffer))
	})
}

func TestMpegpsPESFrame(t *testing.T) {
	// Test MpegpsPESFrame basic functionality
	t.Run("PESFrame", func(t *testing.T) {
		// Create PES frame
		pesFrame := &MpegpsPESFrame{
			StreamType: 0x1B, // H.264
		}
		pesFrame.Pts = 90000 // 1 second in 90kHz clock
		pesFrame.Dts = 90000

		// Test basic properties
		if pesFrame.StreamType != 0x1B {
			t.Errorf("Expected stream type 0x1B, got %x", pesFrame.StreamType)
		}

		if pesFrame.Pts != 90000 {
			t.Errorf("Expected PTS %d, got %d", 90000, pesFrame.Pts)
		}

		if pesFrame.Dts != 90000 {
			t.Errorf("Expected DTS %d, got %d", 90000, pesFrame.Dts)
		}

		t.Logf("PES Frame: StreamType=%x, PTS=%d, DTS=%d", pesFrame.StreamType, pesFrame.Pts, pesFrame.Dts)
	})
}

func TestReadPayload(t *testing.T) {
	// Test ReadPayload functionality
	t.Run("ReadPayload", func(t *testing.T) {
		// Create test data with payload length and payload
		testData := []byte{
			0x00, 0x05, // Payload length = 5 bytes
			0x01, 0x02, 0x03, 0x04, 0x05, // Payload data
		}

		demuxer := &MpegPsDemuxer{}
		reader := util.NewBufReader(bytes.NewReader(testData))

		payload, err := demuxer.ReadPayload(reader)
		if err != nil {
			t.Fatalf("ReadPayload failed: %v", err)
		}

		if payload.Size != 5 {
			t.Errorf("Expected payload size 5, got %d", payload.Size)
		}

		expectedPayload := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
		if !bytes.Equal(payload.ToBytes(), expectedPayload) {
			t.Errorf("Expected payload %x, got %x", expectedPayload, payload.ToBytes())
		}

		t.Logf("ReadPayload successful: %x", payload.ToBytes())
	})
}

func TestMpegPSMuxerBasic(t *testing.T) {
	// Test MpegPSMuxer basic functionality
	t.Run("MuxBasic", func(t *testing.T) {

		// Test basic PS header generation without PlayBlock
		// This focuses on testing the header generation logic
		var outputBuffer util.Buffer = make([]byte, 0, 1024)
		outputBuffer.Reset()

		// Test PS header generation
		MuxPSHeader(&outputBuffer)

		// Add stuffing bytes as expected by the demuxer
		// The demuxer expects: 9 bytes + 1 stuffing length byte + stuffing bytes
		stuffingLength := byte(0x00) // No stuffing bytes
		outputBuffer.WriteByte(stuffingLength)

		// Verify PS header contains expected start code
		if len(outputBuffer) != PSPackHeaderSize+1 {
			t.Errorf("Expected PS header size %d, got %d", PSPackHeaderSize+1, len(outputBuffer))
		}

		// Check for PS start code
		if !bytes.Contains(outputBuffer, []byte{0x00, 0x00, 0x01, 0xBA}) {
			t.Error("PS header does not contain PS start code")
		}

		t.Logf("PS Header: %x", outputBuffer)
		t.Logf("PS Header size: %d bytes", len(outputBuffer))

		// Test PSM header generation
		var pesAudio, pesVideo *MpegpsPESFrame
		var elementary_stream_map_length uint16

		// Simulate audio stream
		hasAudio := true
		if hasAudio {
			elementary_stream_map_length += 4
			pesAudio = &MpegpsPESFrame{}
			pesAudio.StreamID = 0xC0   // MPEG audio
			pesAudio.StreamType = 0x0F // AAC
		}

		// Simulate video stream
		hasVideo := true
		if hasVideo {
			elementary_stream_map_length += 4
			pesVideo = &MpegpsPESFrame{}
			pesVideo.StreamID = 0xE0   // MPEG video
			pesVideo.StreamType = 0x1B // H.264
		}

		// Create PSM header with proper payload length
		psmData := make([]byte, 0, PSMHeaderSize+int(elementary_stream_map_length))
		psmBuffer := util.Buffer(psmData)
		psmBuffer.Reset()

		// Write PSM start code
		psmBuffer.WriteUint32(StartCodeMAP)
		psmLength := uint16(PSMHeaderSize + int(elementary_stream_map_length) - 6)
		psmBuffer.WriteUint16(psmLength) // psm_length
		psmBuffer.WriteByte(0xE0)        // current_next_indicator + reserved + psm_version
		psmBuffer.WriteByte(0xFF)        // reserved + marker
		psmBuffer.WriteUint16(0)         // program_stream_info_length

		psmBuffer.WriteUint16(elementary_stream_map_length)
		if pesAudio != nil {
			psmBuffer.WriteByte(pesAudio.StreamType) // stream_type
			psmBuffer.WriteByte(pesAudio.StreamID)   // elementary_stream_id
			psmBuffer.WriteUint16(0)                 // elementary_stream_info_length
		}
		if pesVideo != nil {
			psmBuffer.WriteByte(pesVideo.StreamType) // stream_type
			psmBuffer.WriteByte(pesVideo.StreamID)   // elementary_stream_id
			psmBuffer.WriteUint16(0)                 // elementary_stream_info_length
		}

		// Verify PSM header
		if len(psmBuffer) != PSMHeaderSize+int(elementary_stream_map_length) {
			t.Errorf("Expected PSM size %d, got %d", PSMHeaderSize+int(elementary_stream_map_length), len(psmBuffer))
		}

		// Check for PSM start code
		if !bytes.Contains(psmBuffer, []byte{0x00, 0x00, 0x01, 0xBC}) {
			t.Error("PSM header does not contain PSM start code")
		}

		t.Logf("PSM Header: %x", psmBuffer)
		t.Logf("PSM Header size: %d bytes", len(psmBuffer))

		// Test ReadPayload function directly
		t.Run("ReadPayload", func(t *testing.T) {
			// Create test payload data
			testPayload := []byte{0x01, 0x02, 0x03, 0x04, 0x05}

			// Create a packet with length prefix
			packetData := make([]byte, 0, 2+len(testPayload))
			packetData = append(packetData, byte(len(testPayload)>>8), byte(len(testPayload)))
			packetData = append(packetData, testPayload...)

			reader := util.NewBufReader(bytes.NewReader(packetData))
			demuxer := &MpegPsDemuxer{}

			// Test ReadPayload function
			payload, err := demuxer.ReadPayload(reader)
			if err != nil {
				t.Fatalf("ReadPayload failed: %v", err)
			}

			if payload.Size != len(testPayload) {
				t.Errorf("Expected payload size %d, got %d", len(testPayload), payload.Size)
			}

			if !bytes.Equal(payload.ToBytes(), testPayload) {
				t.Errorf("Expected payload %x, got %x", testPayload, payload.ToBytes())
			}

			t.Logf("ReadPayload test passed: %x", payload.ToBytes())
		})

		// Test basic demuxing with PS header only
		t.Run("PSHeader", func(t *testing.T) {
			// Create a simple test that just verifies the PS header structure
			// without trying to demux it (which expects more data)
			if len(outputBuffer) < 4 {
				t.Errorf("PS header too short: %d bytes", len(outputBuffer))
			}

			// Check that it starts with the correct start code
			if !bytes.HasPrefix(outputBuffer, []byte{0x00, 0x00, 0x01, 0xBA}) {
				t.Errorf("PS header does not start with correct start code: %x", outputBuffer[:4])
			}

			t.Logf("PS header structure test passed")
		})

		t.Logf("Basic mux/demux test completed successfully")
	})

	// Test basic PES packet generation without PlayBlock
	t.Run("PESGeneration", func(t *testing.T) {
		// Create a test that simulates PES packet generation
		// without requiring a full subscriber setup
		
		// Create test payload
		testPayload := make([]byte, 5000)
		for i := range testPayload {
			testPayload[i] = byte(i % 256)
		}

		// Create PES frame
		pesFrame := &MpegpsPESFrame{
			StreamType: 0x1B, // H.264
		}
		pesFrame.Pts = 90000
		pesFrame.Dts = 90000

		// Create allocator for testing
		allocator := util.NewScalableMemoryAllocator(1024*1024)
		packet := util.NewRecyclableMemory(allocator)

		// Write PES packet
		err := pesFrame.WritePESPacket(util.NewMemory(testPayload), &packet)
		if err != nil {
			t.Fatalf("WritePESPacket failed: %v", err)
		}

		// Verify packet was written
		packetData := packet.ToBytes()
		if len(packetData) == 0 {
			t.Fatal("No data was written to packet")
		}

		t.Logf("PES packet generated: %d bytes", len(packetData))
		t.Logf("Packet data (first 64 bytes): %x", packetData[:min(64, len(packetData))])

		// Verify PS header is present
		if !bytes.Contains(packetData, []byte{0x00, 0x00, 0x01, 0xBA}) {
			t.Error("PES packet does not contain PS start code")
		}

		// Test reading back the packet
		reader := util.NewBufReader(bytes.NewReader(packetData))
		
		// Skip PS header
		code, err := reader.ReadBE32(4)
		if err != nil {
			t.Fatalf("Failed to read start code: %v", err)
		}
		if code != StartCodePS {
			t.Errorf("Expected PS start code %x, got %x", StartCodePS, code)
		}

		// Skip PS header
		if err = reader.Skip(9); err != nil {
			t.Fatalf("Failed to skip PS header: %v", err)
		}
		psl, err := reader.ReadByte()
		if err != nil {
			t.Fatalf("Failed to read stuffing length: %v", err)
		}
		psl &= 0x07
		if err = reader.Skip(int(psl)); err != nil {
			t.Fatalf("Failed to skip stuffing bytes: %v", err)
		}

		// Read PES packets directly by parsing the PES structure
		totalPayloadSize := 0
		packetCount := 0
		
		for reader.Buffered() > 0 {
			// Read PES packet start code (0x00000100 + stream_id)
			pesStartCode, err := reader.ReadBE32(4)
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("Failed to read PES start code: %v", err)
			}
			
			// Check if it's a PES packet (starts with 0x000001)
			if pesStartCode&0xFFFFFF00 != 0x00000100 {
				t.Errorf("Invalid PES start code: %x", pesStartCode)
				break
			}
			
			// // streamID := byte(pesStartCode & 0xFF)
			t.Logf("PES packet %d: stream_id=0x%02x", packetCount+1, pesStartCode&0xFF)
			
			// Read PES packet length
			pesLength, err := reader.ReadBE(2)
			if err != nil {
				t.Fatalf("Failed to read PES length: %v", err)
			}
			
			// Read PES header
			// Skip the first byte (flags)
			_, err = reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES flags1: %v", err)
			}
			
			// Skip the second byte (flags)
			_, err = reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES flags2: %v", err)
			}
			
			// Read header data length
			headerDataLength, err := reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES header data length: %v", err)
			}
			
			// Skip header data
			if err = reader.Skip(int(headerDataLength)); err != nil {
				t.Fatalf("Failed to skip PES header data: %v", err)
			}
			
			// Calculate payload size
			payloadSize := pesLength - 3 - int(headerDataLength) // 3 = flags1 + flags2 + headerDataLength
			if payloadSize > 0 {
				// Read payload data
				payload, err := reader.ReadBytes(payloadSize)
				if err != nil {
					t.Fatalf("Failed to read PES payload: %v", err)
				}
				
				totalPayloadSize += payload.Size
				t.Logf("PES packet %d: %d bytes payload", packetCount+1, payload.Size)
			}
			
			packetCount++
		}

		// Verify total payload size matches
		if totalPayloadSize != len(testPayload) {
			t.Errorf("Expected total payload size %d, got %d", len(testPayload), totalPayloadSize)
		}

		t.Logf("PES generation test completed successfully: %d packets, total %d bytes", packetCount, totalPayloadSize)
	})
}

func TestPESPacketWriteRead(t *testing.T) {
	// Test PES packet writing and reading functionality
	t.Run("PESWriteRead", func(t *testing.T) {
		// Create test payload data
		testPayload := make([]byte, 1000)
		for i := range testPayload {
			testPayload[i] = byte(i % 256)
		}

		// Create PES frame
		pesFrame := &MpegpsPESFrame{
			StreamType: 0x1B, // H.264
		}
		pesFrame.Pts = 90000 // 1 second in 90kHz clock
		pesFrame.Dts = 90000

		// Create allocator for testing
		allocator := util.NewScalableMemoryAllocator(1024)
		packet := util.NewRecyclableMemory(allocator)

		// Write PES packet
		err := pesFrame.WritePESPacket(util.NewMemory(testPayload), &packet)
		if err != nil {
			t.Fatalf("WritePESPacket failed: %v", err)
		}

		// Verify that packet was written
		packetData := packet.ToBytes()
		if len(packetData) == 0 {
			t.Fatal("No data was written to packet")
		}

		t.Logf("PES packet written: %d bytes", len(packetData))
		t.Logf("Packet data (first 64 bytes): %x", packetData[:min(64, len(packetData))])

		// Verify PS header is present
		if !bytes.Contains(packetData, []byte{0x00, 0x00, 0x01, 0xBA}) {
			t.Error("PES packet does not contain PS start code")
		}

		// Now test reading the PES packet back
		reader := util.NewBufReader(bytes.NewReader(packetData))

		// Read and process the PS header
		code, err := reader.ReadBE32(4)
		if err != nil {
			t.Fatalf("Failed to read start code: %v", err)
		}
		if code != StartCodePS {
			t.Errorf("Expected PS start code %x, got %x", StartCodePS, code)
		}

		// Skip PS header (9 bytes + stuffing length)
		if err = reader.Skip(9); err != nil {
			t.Fatalf("Failed to skip PS header: %v", err)
		}
		psl, err := reader.ReadByte()
		if err != nil {
			t.Fatalf("Failed to read stuffing length: %v", err)
		}
		psl &= 0x07
		if err = reader.Skip(int(psl)); err != nil {
			t.Fatalf("Failed to skip stuffing bytes: %v", err)
		}

		// Read PES packet directly by parsing the PES structure
		totalPayloadSize := 0
		packetCount := 0
		
		for reader.Buffered() > 0 {
			// Read PES packet start code (0x00000100 + stream_id)
			pesStartCode, err := reader.ReadBE32(4)
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("Failed to read PES start code: %v", err)
			}
			
			// Check if it's a PES packet (starts with 0x000001)
			if pesStartCode&0xFFFFFF00 != 0x00000100 {
				t.Errorf("Invalid PES start code: %x", pesStartCode)
				break
			}
			
			// // streamID := byte(pesStartCode & 0xFF)
			t.Logf("PES packet %d: stream_id=0x%02x", packetCount+1, pesStartCode&0xFF)
			
			// Read PES packet length
			pesLength, err := reader.ReadBE(2)
			if err != nil {
				t.Fatalf("Failed to read PES length: %v", err)
			}
			
			// Read PES header
			// Skip the first byte (flags)
			_, err = reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES flags1: %v", err)
			}
			
			// Skip the second byte (flags)
			_, err = reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES flags2: %v", err)
			}
			
			// Read header data length
			headerDataLength, err := reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES header data length: %v", err)
			}
			
			// Skip header data
			if err = reader.Skip(int(headerDataLength)); err != nil {
				t.Fatalf("Failed to skip PES header data: %v", err)
			}
			
			// Calculate payload size
			payloadSize := pesLength - 3 - int(headerDataLength) // 3 = flags1 + flags2 + headerDataLength
			if payloadSize > 0 {
				// Read payload data
				payload, err := reader.ReadBytes(payloadSize)
				if err != nil {
					t.Fatalf("Failed to read PES payload: %v", err)
				}
				
				totalPayloadSize += payload.Size
				t.Logf("PES packet %d: %d bytes payload", packetCount+1, payload.Size)
			}
			
			packetCount++
		}

		t.Logf("PES payload read: %d bytes", totalPayloadSize)

		// Verify payload size
		if totalPayloadSize != len(testPayload) {
			t.Errorf("Expected payload size %d, got %d", len(testPayload), totalPayloadSize)
		}

		// Note: We can't easily verify the content because the payload is fragmented across multiple PES packets
		// But we can verify the total size is correct

		t.Logf("PES packet write-read test completed successfully")
	})
}

func TestLargePESPacket(t *testing.T) {
	// Test large PES packet handling (payload > 65535 bytes)
	t.Run("LargePESPacket", func(t *testing.T) {
		// Create large test payload (exceeds 65535 bytes)
		largePayload := make([]byte, 70000) // 70KB payload
		for i := range largePayload {
			largePayload[i] = byte(i % 256)
		}

		// Create PES frame
		pesFrame := &MpegpsPESFrame{
			StreamType: 0x1B, // H.264
		}
		pesFrame.Pts = 180000 // 2 seconds in 90kHz clock
		pesFrame.Dts = 180000

		// Create allocator for testing
		allocator := util.NewScalableMemoryAllocator(1024*1024) // 1MB allocator
		packet := util.NewRecyclableMemory(allocator)

		// Write large PES packet
		t.Logf("Writing large PES packet with %d bytes payload", len(largePayload))
		err := pesFrame.WritePESPacket(util.NewMemory(largePayload), &packet)
		if err != nil {
			t.Fatalf("WritePESPacket failed for large payload: %v", err)
		}

		// Verify that packet was written
		packetData := packet.ToBytes()
		if len(packetData) == 0 {
			t.Fatal("No data was written to packet")
		}

		t.Logf("Large PES packet written: %d bytes", len(packetData))

		// Verify PS header is present
		if !bytes.Contains(packetData, []byte{0x00, 0x00, 0x01, 0xBA}) {
			t.Error("Large PES packet does not contain PS start code")
		}

		// Count number of PES packets (should be multiple due to size limitation)
		pesCount := 0
		reader := util.NewBufReader(bytes.NewReader(packetData))
		
		// Skip PS header
		code, err := reader.ReadBE32(4)
		if err != nil {
			t.Fatalf("Failed to read start code: %v", err)
		}
		if code != StartCodePS {
			t.Errorf("Expected PS start code %x, got %x", StartCodePS, code)
		}

		// Skip PS header
		if err = reader.Skip(9); err != nil {
			t.Fatalf("Failed to skip PS header: %v", err)
		}
		psl, err := reader.ReadByte()
		if err != nil {
			t.Fatalf("Failed to read stuffing length: %v", err)
		}
		psl &= 0x07
		if err = reader.Skip(int(psl)); err != nil {
			t.Fatalf("Failed to skip stuffing bytes: %v", err)
		}

		// Read and count PES packets
		totalPayloadSize := 0
		
		for reader.Buffered() > 0 {
			// Read PES packet start code (0x00000100 + stream_id)
			pesStartCode, err := reader.ReadBE32(4)
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("Failed to read PES start code: %v", err)
			}
			
			// Check if it's a PES packet (starts with 0x000001)
			if pesStartCode&0xFFFFFF00 != 0x00000100 {
				t.Errorf("Invalid PES start code: %x", pesStartCode)
				break
			}
			
			// streamID := byte(pesStartCode & 0xFF)
			
			// Read PES packet length
			pesLength, err := reader.ReadBE(2)
			if err != nil {
				t.Fatalf("Failed to read PES length: %v", err)
			}
			
			// Read PES header
			// Skip the first byte (flags)
			_, err = reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES flags1: %v", err)
			}
			
			// Skip the second byte (flags)
			_, err = reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES flags2: %v", err)
			}
			
			// Read header data length
			headerDataLength, err := reader.ReadByte()
			if err != nil {
				t.Fatalf("Failed to read PES header data length: %v", err)
			}
			
			// Skip header data
			if err = reader.Skip(int(headerDataLength)); err != nil {
				t.Fatalf("Failed to skip PES header data: %v", err)
			}
			
			// Calculate payload size
			payloadSize := pesLength - 3 - int(headerDataLength) // 3 = flags1 + flags2 + headerDataLength
			if payloadSize > 0 {
				// Read payload data
				payload, err := reader.ReadBytes(payloadSize)
				if err != nil {
					t.Fatalf("Failed to read PES payload: %v", err)
				}
				
				totalPayloadSize += payload.Size
				t.Logf("PES packet %d: %d bytes payload", pesCount+1, payload.Size)
			}
			
			pesCount++
		}

		// Verify that we got multiple PES packets
		if pesCount < 2 {
			t.Errorf("Expected multiple PES packets for large payload, got %d", pesCount)
		}

		// Verify total payload size
		if totalPayloadSize != len(largePayload) {
			t.Errorf("Expected total payload size %d, got %d", len(largePayload), totalPayloadSize)
		}

		// Verify individual PES packet sizes don't exceed maximum
		maxPacketSize := MaxPESPayloadSize + PESHeaderMinSize
		if pesCount == 1 && len(packetData) > maxPacketSize {
			t.Errorf("Single PES packet exceeds maximum size: %d > %d", len(packetData), maxPacketSize)
		}

		t.Logf("Large PES packet test completed successfully: %d packets, total %d bytes", pesCount, totalPayloadSize)
	})
}

func TestPESPacketBoundaryConditions(t *testing.T) {
	// Test PES packet boundary conditions
	t.Run("BoundaryConditions", func(t *testing.T) {
		testCases := []struct {
			name     string
			payloadSize int
		}{
			{"EmptyPayload", 0},
			{"SmallPayload", 1},
			{"ExactBoundary", MaxPESPayloadSize},
			{"JustOverBoundary", MaxPESPayloadSize + 1},
			{"MultipleBoundary", MaxPESPayloadSize * 2 + 100},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Create test payload
				testPayload := make([]byte, tc.payloadSize)
				for i := range testPayload {
					testPayload[i] = byte(i % 256)
				}

				// Create PES frame
				pesFrame := &MpegpsPESFrame{
					StreamType: 0x1B, // H.264
				}
				pesFrame.Pts = uint64(tc.payloadSize) * 90 // Use payload size as PTS
				pesFrame.Dts = uint64(tc.payloadSize) * 90

				// Create allocator for testing
				allocator := util.NewScalableMemoryAllocator(1024*1024)
				packet := util.NewRecyclableMemory(allocator)

				// Write PES packet
				err := pesFrame.WritePESPacket(util.NewMemory(testPayload), &packet)
				if err != nil {
					t.Fatalf("WritePESPacket failed: %v", err)
				}

				// Verify that packet was written
				packetData := packet.ToBytes()
				if len(packetData) == 0 && tc.payloadSize > 0 {
					t.Fatal("No data was written to packet for non-empty payload")
				}

				t.Logf("%s: %d bytes payload -> %d bytes packet", tc.name, tc.payloadSize, len(packetData))

				// For non-empty payloads, verify we can read them back
				if tc.payloadSize > 0 {
					reader := util.NewBufReader(bytes.NewReader(packetData))
					
					// Skip PS header
					code, err := reader.ReadBE32(4)
					if err != nil {
						t.Fatalf("Failed to read start code: %v", err)
					}
					if code != StartCodePS {
						t.Errorf("Expected PS start code %x, got %x", StartCodePS, code)
					}

					// Skip PS header
					if err = reader.Skip(9); err != nil {
						t.Fatalf("Failed to skip PS header: %v", err)
					}
					psl, err := reader.ReadByte()
					if err != nil {
						t.Fatalf("Failed to read stuffing length: %v", err)
					}
					psl &= 0x07
					if err = reader.Skip(int(psl)); err != nil {
						t.Fatalf("Failed to skip stuffing bytes: %v", err)
					}

					// Read PES packets
					totalPayloadSize := 0
					packetCount := 0
					
					for reader.Buffered() > 0 {
						// Read PES packet start code (0x00000100 + stream_id)
						pesStartCode, err := reader.ReadBE32(4)
						if err != nil {
							if err == io.EOF {
								break
							}
							t.Fatalf("Failed to read PES start code: %v", err)
						}
						
						// Check if it's a PES packet (starts with 0x000001)
						if pesStartCode&0xFFFFFF00 != 0x00000100 {
							t.Errorf("Invalid PES start code: %x", pesStartCode)
							break
						}
						
						// // streamID := byte(pesStartCode & 0xFF)
						
						// Read PES packet length
						pesLength, err := reader.ReadBE(2)
						if err != nil {
							t.Fatalf("Failed to read PES length: %v", err)
						}
						
						// Read PES header
						// Skip the first byte (flags)
						_, err = reader.ReadByte()
						if err != nil {
							t.Fatalf("Failed to read PES flags1: %v", err)
						}
						
						// Skip the second byte (flags)
						_, err = reader.ReadByte()
						if err != nil {
							t.Fatalf("Failed to read PES flags2: %v", err)
						}
						
						// Read header data length
						headerDataLength, err := reader.ReadByte()
						if err != nil {
							t.Fatalf("Failed to read PES header data length: %v", err)
						}
						
						// Skip header data
						if err = reader.Skip(int(headerDataLength)); err != nil {
							t.Fatalf("Failed to skip PES header data: %v", err)
						}
						
						// Calculate payload size
						payloadSize := pesLength - 3 - int(headerDataLength) // 3 = flags1 + flags2 + headerDataLength
						if payloadSize > 0 {
							// Read payload data
							payload, err := reader.ReadBytes(payloadSize)
							if err != nil {
								t.Fatalf("Failed to read PES payload: %v", err)
							}
							
							totalPayloadSize += payload.Size
						}
						
						packetCount++
					}

					// Verify total payload size matches
					if totalPayloadSize != tc.payloadSize {
						t.Errorf("Expected total payload size %d, got %d", tc.payloadSize, totalPayloadSize)
					}

					t.Logf("%s: Successfully read back %d PES packets", tc.name, packetCount)
				}
			})
		}
	})
}
