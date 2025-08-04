package pkg

import (
	"bytes"
	_ "embed"
	"math/rand"
	"testing"

	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

func bytesFromMemory(m util.Memory) []byte {
	if m.Size == 0 {
		return nil
	}
	out := make([]byte, 0, m.Size)
	for _, b := range m.Buffers {
		out = append(out, b...)
	}
	return out
}

func TestAnnexBReader_ReadNALU_Basic(t *testing.T) {

	var reader AnnexBReader

	// 3 个 NALU，分别使用 4 字节、3 字节、4 字节起始码
	expected1 := []byte{0x67, 0x42, 0x00, 0x1E}
	expected2 := []byte{0x68, 0xCE, 0x3C, 0x80}
	expected3 := []byte{0x65, 0x88, 0x84, 0x00}

	buf := append([]byte{0x00, 0x00, 0x00, 0x01}, expected1...)
	buf = append(buf, append([]byte{0x00, 0x00, 0x01}, expected2...)...)
	buf = append(buf, append([]byte{0x00, 0x00, 0x00, 0x01}, expected3...)...)

	reader.AppendBuffer(append(buf, codec.NALU_Delimiter2[:]...))

	// 读取并校验 3 个 NALU（不包含起始码）
	var n util.Memory
	if err := reader.ReadNALU(nil, &n); err != nil {
		t.Fatalf("read nalu 1: %v", err)
	}
	if !bytes.Equal(bytesFromMemory(n), expected1) {
		t.Fatalf("nalu1 mismatch")
	}

	n = util.Memory{}
	if err := reader.ReadNALU(nil, &n); err != nil {
		t.Fatalf("read nalu 2: %v", err)
	}
	if !bytes.Equal(bytesFromMemory(n), expected2) {
		t.Fatalf("nalu2 mismatch")
	}

	n = util.Memory{}
	if err := reader.ReadNALU(nil, &n); err != nil {
		t.Fatalf("read nalu 3: %v", err)
	}
	if !bytes.Equal(bytesFromMemory(n), expected3) {
		t.Fatalf("nalu3 mismatch")
	}

	// 再读一次应无更多起始码，返回 nil 错误且长度为 0
	if err := reader.ReadNALU(nil, &n); err != nil {
		t.Fatalf("expected nil error when no more nalu, got: %v", err)
	}
	if reader.Length != 4 {
		t.Fatalf("expected length 0 after reading all, got %d", reader.Length)
	}
}

func TestAnnexBReader_AppendBuffer_MultiChunk_Random(t *testing.T) {

	var reader AnnexBReader

	rng := rand.New(rand.NewSource(1)) // 固定种子，保证可复现

	// 生成随机 NALU（仅负载部分），并构造 AnnexB 数据（随机 3/4 字节起始码）
	numNALU := 12
	expectedPayloads := make([][]byte, 0, numNALU)
	fullStream := make([]byte, 0, 1024)

	for i := 0; i < numNALU; i++ {
		payloadLen := 1 + rng.Intn(32)
		payload := make([]byte, payloadLen)
		for j := 0; j < payloadLen; j++ {
			payload[j] = byte(rng.Intn(256))
		}
		expectedPayloads = append(expectedPayloads, payload)

		if rng.Intn(2) == 0 {
			fullStream = append(fullStream, 0x00, 0x00, 0x01)
		} else {
			fullStream = append(fullStream, 0x00, 0x00, 0x00, 0x01)
		}
		fullStream = append(fullStream, payload...)
	}
	fullStream = append(fullStream, codec.NALU_Delimiter2[:]...) // 结尾加个起始码，方便读取到最后一个 NALU
	// 随机切割为多段并 AppendBuffer
	for i := 0; i < len(fullStream); {
		// 每段长度 1..7 字节（或剩余长度）
		maxStep := 7
		remain := len(fullStream) - i
		step := 1 + rng.Intn(maxStep)
		if step > remain {
			step = remain
		}
		reader.AppendBuffer(fullStream[i : i+step])
		i += step
	}

	// 依次读取并校验
	for idx, expected := range expectedPayloads {
		var n util.Memory
		if err := reader.ReadNALU(nil, &n); err != nil {
			t.Fatalf("read nalu %d: %v", idx+1, err)
		}
		got := bytesFromMemory(n)
		if !bytes.Equal(got, expected) {
			t.Fatalf("nalu %d mismatch: expected %d bytes, got %d bytes", idx+1, len(expected), len(got))
		}
	}

	// 没有更多 NALU
	var n util.Memory
	if err := reader.ReadNALU(nil, &n); err != nil {
		t.Fatalf("expected nil error when no more nalu, got: %v", err)
	}
}

// 起始码跨越两个缓冲区的情况测试（例如 00 00 | 00 01）
func TestAnnexBReader_StartCodeAcrossBuffers(t *testing.T) {
	var reader AnnexBReader
	// 构造一个 4 字节起始码被拆成两段的情况，后跟一个短 payload
	reader.AppendBuffer([]byte{0x00, 0x00})
	reader.AppendBuffer([]byte{0x00})
	reader.AppendBuffer([]byte{0x01, 0x11, 0x22, 0x33}) // payload: 11 22 33
	reader.AppendBuffer(codec.NALU_Delimiter2[:])
	var n util.Memory
	if err := reader.ReadNALU(nil, &n); err != nil {
		t.Fatalf("read nalu: %v", err)
	}
	got := bytesFromMemory(n)
	expected := []byte{0x11, 0x22, 0x33}
	if !bytes.Equal(got, expected) {
		t.Fatalf("payload mismatch: expected %v got %v", expected, got)
	}
}

//go:embed test.h264
var annexbH264Sample []byte

var clipSizesH264 = [...]int{7823, 7157, 5137, 6268, 5958, 4573, 5661, 5589, 3917, 5207, 5347, 4111, 4755, 5199, 3761, 5014, 4981, 3736, 5075, 4889, 3739, 4701, 4655, 3471, 4086, 4428, 3309, 4388, 28, 8, 63974, 63976, 37544, 4945, 6525, 6974, 4874, 6317, 6141, 4455, 5833, 4105, 5407, 5479, 3741, 5142, 4939, 3745, 4945, 4857, 3518, 4624, 4930, 3649, 4846, 5020, 3293, 4588, 4571, 3430, 4844, 4822, 21223, 8461, 7188, 4882, 6108, 5870, 4432, 5389, 5466, 3726}

func TestAnnexBReader_EmbeddedAnnexB_H265(t *testing.T) {
	var reader AnnexBReader
	offset := 0
	for _, size := range clipSizesH264 {
		reader.AppendBuffer(annexbH264Sample[offset : offset+size])
		offset += size
		var nalu util.Memory
		if err := reader.ReadNALU(nil, &nalu); err != nil {
			t.Fatalf("read nalu: %v", err)
		} else {
			t.Logf("read nalu: %d bytes", nalu.Size)
			if nalu.Size > 0 {
				tryH264Type := codec.ParseH264NALUType(nalu.Buffers[0][0])
				t.Logf("tryH264Type: %d", tryH264Type)
			}
		}
	}
}
