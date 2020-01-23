package rtmp

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
)

const (
	C1S1_SIZE = 1536

	C1S1_TIME_SIZE    = 4
	C1S1_VERSION_SIZE = 4

	C1S1_DIGEST_SIZE        = 764
	C1S1_DIGEST_OFFSET_SIZE = 4
	C1S1_DIGEST_OFFSET_MAX  = 764 - 32 - 4
	C1S1_DIGEST_DATA_SIZE   = 32

	C1S1_KEY_SIZE        = 764
	C1S1_KEY_OFFSET_SIZE = 4
	C1S1_KEY_OFFSET_MAX  = 764 - 128 - 4
	C1S1_KEY_DATA_SIZE   = 128

	RTMP_HANDSHAKE_VERSION = 0x03
)

var (
	FMS_KEY = []byte{
		0x47, 0x65, 0x6e, 0x75, 0x69, 0x6e, 0x65, 0x20,
		0x41, 0x64, 0x6f, 0x62, 0x65, 0x20, 0x46, 0x6c,
		0x61, 0x73, 0x68, 0x20, 0x4d, 0x65, 0x64, 0x69,
		0x61, 0x20, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72,
		0x20, 0x30, 0x30, 0x31, // Genuine Adobe Flash Media Server 001
		0xf0, 0xee, 0xc2, 0x4a, 0x80, 0x68, 0xbe, 0xe8,
		0x2e, 0x00, 0xd0, 0xd1, 0x02, 0x9e, 0x7e, 0x57,
		0x6e, 0xec, 0x5d, 0x2d, 0x29, 0x80, 0x6f, 0xab,
		0x93, 0xb8, 0xe6, 0x36, 0xcf, 0xeb, 0x31, 0xae,
	} // 68
	FP_KEY = []byte{
		0x47, 0x65, 0x6E, 0x75, 0x69, 0x6E, 0x65, 0x20,
		0x41, 0x64, 0x6F, 0x62, 0x65, 0x20, 0x46, 0x6C,
		0x61, 0x73, 0x68, 0x20, 0x50, 0x6C, 0x61, 0x79,
		0x65, 0x72, 0x20, 0x30, 0x30, 0x31, /* Genuine Adobe Flash Player 001 */
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8,
		0x2E, 0x00, 0xD0, 0xD1, 0x02, 0x9E, 0x7E, 0x57,
		0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	} // 62
)

// C0 S0 (1 byte) : 版本号

// C1 S1 :
// Time (4 bytes)
// Zero (4 bytes) -> 这个字段必须都是0.如果不是0,代表要使用complex handshack
// Random data (128 Bytes)

// C2 S2 : 参考C1 S1

func ReadBuf(r io.Reader, length int) (buf []byte) {
	buf = make([]byte, length)
	io.ReadFull(r, buf)
	return
}

func Handshake(brw *bufio.ReadWriter) error {
	C0C1 := ReadBuf(brw, 1536+1)
	if C0C1[0] != RTMP_HANDSHAKE_VERSION {
		return errors.New("C0 Error")
	}

	if len(C0C1[1:]) != 1536 {
		return errors.New("C1 Error")
	}

	C1 := make([]byte, 1536)
	copy(C1, C0C1[1:])
	temp := C1[4] & 0xff

	if temp == 0 {
		return simple_handshake(brw, C1)
	}

	return complex_handshake(brw, C1)
}

func simple_handshake(brw *bufio.ReadWriter, C1 []byte) error {
	var S0 byte
	S0 = 0x03
	S1 := make([]byte, 1536-4)
	S2 := C1
	S1_Time := uint32(0)

	buf := new(bytes.Buffer)
	buf.WriteByte(S0)
	binary.Write(buf, binary.BigEndian, S1_Time)
	buf.Write(S1)
	buf.Write(S2)

	brw.Write(buf.Bytes())
	brw.Flush() // Don't forget to flush

	ReadBuf(brw, 1536)
	return nil
}

func complex_handshake(brw *bufio.ReadWriter, C1 []byte) error {
	// 验证客户端,digest偏移位置和scheme由客户端定.
	scheme, challenge, digest, ok, err := validateClient(C1)
	if err != nil {
		return err
	}

	fmt.Sprintf("digested handshake, scheme : %v\nchallenge : %v\ndigest : %v\nok : %v\nerr : %v\n", scheme, challenge, digest, ok, err)

	if !ok {
		return errors.New("validateClient failed")
	}

	// s0
	var S0 byte
	S0 = 0x03

	// s1
	S1 := create_S1()
	S1_Digest_Offset := scheme_Digest_Offset(S1, scheme)
	S1_Part1 := S1[:S1_Digest_Offset]
	S1_Part2 := S1[S1_Digest_Offset+C1S1_DIGEST_DATA_SIZE:]

	// s1 part1 + part2
	buf := new(bytes.Buffer)
	buf.Write(S1_Part1)
	buf.Write(S1_Part2)
	S1_Part1_Part2 := buf.Bytes()

	// s1 digest
	tmp_Hash, err := HMAC_SHA256(S1_Part1_Part2, FMS_KEY[:36])
	if err != nil {
		return err
	}

	// incomplete s1
	copy(S1[S1_Digest_Offset:], tmp_Hash)

	// s2
	S2_Random := cerate_S2()

	tmp_Hash, err = HMAC_SHA256(digest, FMS_KEY[:68])
	if err != nil {
		return err
	}

	// s2 digest
	S2_Digest, err := HMAC_SHA256(S2_Random, tmp_Hash)
	if err != nil {
		return err
	}

	buffer := new(bytes.Buffer)
	buffer.WriteByte(S0)
	buffer.Write(S1)
	buffer.Write(S2_Random)
	buffer.Write(S2_Digest)

	brw.Write(buffer.Bytes())
	brw.Flush()

	ReadBuf(brw, 1536)
	return nil
}

func validateClient(C1 []byte) (scheme int, challenge []byte, digest []byte, ok bool, err error) {
	scheme, challenge, digest, ok, err = clientScheme(C1, 1)
	if ok {
		return scheme, challenge, digest, ok, nil
	}

	scheme, challenge, digest, ok, err = clientScheme(C1, 0)
	if ok {
		return scheme, challenge, digest, ok, nil
	}

	return scheme, challenge, digest, ok, errors.New("Client scheme error")
}

func clientScheme(C1 []byte, schem int) (scheme int, challenge []byte, digest []byte, ok bool, err error) {
	digest_offset := -1
	key_offset := -1

	if schem == 0 {
		digest_offset = scheme0_Digest_Offset(C1)
		key_offset = scheme0_Key_Offset(C1)
	} else if schem == 1 {
		digest_offset = scheme1_Digest_Offset(C1)
		key_offset = scheme1_Key_Offset(C1)
	}

	// digest
	c1_Part1 := C1[:digest_offset]
	c1_Part2 := C1[digest_offset+C1S1_DIGEST_DATA_SIZE:]
	digest = C1[digest_offset : digest_offset+C1S1_DIGEST_DATA_SIZE]

	// part1 + part2
	buf := new(bytes.Buffer)
	buf.Write(c1_Part1)
	buf.Write(c1_Part2)
	c1_Part1_Part2 := buf.Bytes()

	tmp_Hash, err := HMAC_SHA256(c1_Part1_Part2, FP_KEY[:30])
	if err != nil {
		return 0, nil, nil, false, err
	}

	// ok
	if bytes.Compare(digest, tmp_Hash) == 0 {
		ok = true
	} else {
		ok = false
	}

	// challenge scheme
	challenge = C1[key_offset : key_offset+C1S1_KEY_DATA_SIZE]
	scheme = schem
	return
}

func scheme_Digest_Offset(C1S1 []byte, scheme int) int {
	if scheme == 0 {
		return scheme0_Digest_Offset(C1S1)
	} else if scheme == 1 {
		return scheme1_Digest_Offset(C1S1)
	}

	return -1
}

// scheme0:
// time + version + digest 										  + key
// time + version + [offset + random + digest-data + random-data] + key
// 4	+ 4 	  + [4		+ offset + 32		   + 728-offset ] + 764
// 4 	+ 4		  + 764											  + 764
// 0 <= scheme0_digest_offset <= 728 == 764 - 32 - 4
// 如果digest.offset == 3,那么digest[7~38]为digest.digest-data,如果offset == 728, 那么digest[732~763]为digest-data)
func scheme0_Digest_Offset(C1S1 []byte) int {
	scheme0_digest_offset := int(C1S1[8]&0xff) + int(C1S1[9]&0xff) + int(C1S1[10]&0xff) + int(C1S1[11]&0xff)

	scheme0_digest_offset = (scheme0_digest_offset % C1S1_DIGEST_OFFSET_MAX) + C1S1_TIME_SIZE + C1S1_VERSION_SIZE + C1S1_DIGEST_OFFSET_SIZE
	if scheme0_digest_offset+32 >= C1S1_SIZE {
		// digest error
		// digest 数据超出1536.
	}

	return scheme0_digest_offset
}

// key:
// random-data + key-data + random-data 		+ offset
// offset	   + 128	  +	764-offset-128-4	+ 4
// 0 <= scheme0_key_offset <= 632 == 764 - 128 - 4
// 如果key.offset == 3, 那么key[3~130]为key-data,这个位置是相对于key结构的第0个字节开始
func scheme0_Key_Offset(C1S1 []byte) int {
	scheme0_key_offset := int(C1S1[1532]) + int(C1S1[1533]) + int(C1S1[1534]) + int(C1S1[1535])

	scheme0_key_offset = (scheme0_key_offset % C1S1_KEY_OFFSET_MAX) + C1S1_TIME_SIZE + C1S1_VERSION_SIZE + C1S1_DIGEST_SIZE
	if scheme0_key_offset+128 >= C1S1_SIZE {
		// key error
	}

	return scheme0_key_offset
}

// scheme1:
// time + version + key + digest
// 0 <= scheme1_digest_offset <= 728 == 764 - 32 - 4
func scheme1_Digest_Offset(C1S1 []byte) int {
	scheme1_digest_offset := int(C1S1[772]&0xff) + int(C1S1[773]&0xff) + int(C1S1[774]&0xff) + int(C1S1[775]&0xff)

	scheme1_digest_offset = (scheme1_digest_offset % C1S1_DIGEST_OFFSET_MAX) + C1S1_TIME_SIZE + C1S1_VERSION_SIZE + C1S1_KEY_SIZE + C1S1_DIGEST_OFFSET_SIZE
	if scheme1_digest_offset+32 >= C1S1_SIZE {
		// digest error
	}

	return scheme1_digest_offset
}

// time + version + key + digest
// 0 <= scheme1_key_offset <= 632 == 764 - 128 - 4
func scheme1_Key_Offset(C1S1 []byte) int {
	scheme1_key_offset := int(C1S1[768]) + int(C1S1[769]) + int(C1S1[770]) + int(C1S1[771])

	scheme1_key_offset = (scheme1_key_offset % C1S1_KEY_OFFSET_MAX) + C1S1_TIME_SIZE + C1S1_VERSION_SIZE + C1S1_DIGEST_SIZE
	if scheme1_key_offset+128 >= C1S1_SIZE {
		// key error
	}

	return scheme1_key_offset
}

// HMAC运算利用哈希算法,以一个密钥和一个消息为输入,生成一个消息摘要作为输出
// 哈希算法sha256.New, 密钥 key, 消息 message.
func HMAC_SHA256(message []byte, key []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, key)
	_, err := mac.Write(message)
	if err != nil {
		return nil, err
	}
	return mac.Sum(nil), nil
}

func create_S1() []byte {
	s1_Time := []byte{0, 0, 0, 0}
	s1_Version := []byte{1, 1, 1, 1}
	s1_key_Digest := make([]byte, 1536-8)

	for i, _ := range s1_key_Digest {
		s1_key_Digest[i] = byte(rand.Int() % 256)
	}

	buf := new(bytes.Buffer)
	buf.Write(s1_Time)
	buf.Write(s1_Version)
	buf.Write(s1_key_Digest)

	return buf.Bytes()
}

func cerate_S2() []byte {
	s2_Random := make([]byte, 1536-32)

	for i, _ := range s2_Random {
		s2_Random[i] = byte(rand.Int() % 256)
	}

	buf := new(bytes.Buffer)
	buf.Write(s2_Random)

	return buf.Bytes()
}
