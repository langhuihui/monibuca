package method

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"io"
)

type Key struct {
	Key    string
	Iv     string
	EncKey string
	MacKey string
}

func init() {
	RegisterCryptor("stream", newStream)
}

type StreamCryptor struct {
	enckey    []byte
	macKey    []byte
	encrypter *StreamEncrypter `yaml:"-"`
	decrypter *StreamDecrypter `json:"-"`
}

func NewStreamEncrypter(encKey, macKey []byte) (*StreamEncrypter, error) {
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, block.BlockSize())
	_, err = rand.Read(iv)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, iv)
	mac := hmac.New(sha256.New, macKey)

	return &StreamEncrypter{
		Block:  block,
		Stream: stream,
		Mac:    mac,
		IV:     iv,
	}, nil
}
func NewStreamDecrypter(encKey, macKey []byte, meta StreamMeta) (*StreamDecrypter, error) {
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, meta.IV)
	mac := hmac.New(sha256.New, macKey)

	return &StreamDecrypter{
		Block:  block,
		Stream: stream,
		Mac:    mac,
		Meta:   meta,
	}, nil
}

type StreamMeta struct {
	// IV is the initial value for the crypto function
	IV []byte
	// Hash is the sha256 hmac of the stream
	Hash []byte
}

type StreamEncrypter struct {
	Source io.Reader
	Block  cipher.Block
	Stream cipher.Stream
	Mac    hash.Hash
	IV     []byte
}

// StreamDecrypter is a decrypter for a stream of data with authentication
type StreamDecrypter struct {
	Source io.Reader
	Block  cipher.Block
	Stream cipher.Stream
	Mac    hash.Hash
	Meta   StreamMeta
}

// Read encrypts the bytes of the inner reader and places them into p
func (s *StreamEncrypter) Read(p []byte) (int, error) {
	n, readErr := s.Source.Read(p)
	if n > 0 {
		s.Stream.XORKeyStream(p[:n], p[:n])
		err := writeHash(s.Mac, p[:n])
		if err != nil {
			return n, err
		}
		return n, readErr
	}
	return 0, io.EOF
}

// Meta returns the encrypted stream metadata for use in decrypting. This should only be called after the stream is finished
func (s *StreamEncrypter) Meta() StreamMeta {
	return StreamMeta{IV: s.IV, Hash: s.Mac.Sum(nil)}
}

// Read reads bytes from the underlying reader and then decrypts them
func (s *StreamDecrypter) Read(p []byte) (int, error) {
	n, readErr := s.Source.Read(p)
	if n > 0 {
		err := writeHash(s.Mac, p[:n])
		if err != nil {
			return n, err
		}
		s.Stream.XORKeyStream(p[:n], p[:n])
		return n, readErr
	}
	return 0, io.EOF
}

func newStream(cfg Key) (ICryptor, error) {
	var cryptor *StreamCryptor
	if (cfg.EncKey == "") || (cfg.MacKey == "") {
		return nil, errors.New("stream cryptor config not enckey or mackey")
	} else {
		encKey := []byte(cfg.EncKey)
		macKey := []byte(cfg.MacKey)

		encrypter, err := NewStreamEncrypter(encKey, macKey)
		if err != nil {
			return nil, err
		}
		decrypter, err := NewStreamDecrypter(encKey, macKey, encrypter.Meta())
		if err != nil {
			return nil, err
		}
		cryptor = &StreamCryptor{
			enckey:    encKey,
			macKey:    macKey,
			encrypter: encrypter,
			decrypter: decrypter,
		}

	}
	return cryptor, nil
}

func (c *StreamCryptor) Encrypt(origin []byte) ([]byte, error) {
	c.encrypter.Source = bytes.NewReader(origin)
	return io.ReadAll(c.encrypter)
}

func (c *StreamCryptor) Decrypt(encrypted []byte) ([]byte, error) {
	c.decrypter.Source = bytes.NewReader(encrypted)
	return io.ReadAll(c.decrypter)
}

func (c *StreamCryptor) GetKey() string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("%s.%s.%s.%s",
		b64.EncodeToString(c.enckey),
		b64.EncodeToString(c.macKey),
		b64.EncodeToString(c.encrypter.IV),
		b64.EncodeToString(c.encrypter.Mac.Sum(nil)),
	)
}

func writeHash(mac hash.Hash, p []byte) error {
	m, err := mac.Write(p)
	if err != nil {
		return err
	}
	if m != len(p) {
		return errors.New("could not write all bytes to hmac")
	}
	return nil
}
