package pb

import (
	"encoding/json"
	"io"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

var _ runtime.Marshaler = (*TextPlain)(nil)

type TextPlain struct {
}

// ContentType implements runtime.Marshaler.
func (t *TextPlain) ContentType(v interface{}) string {
	return "text/plain"
}

// Marshal implements runtime.Marshaler.
func (t *TextPlain) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// NewDecoder implements runtime.Marshaler.
func (t *TextPlain) NewDecoder(r io.Reader) runtime.Decoder {
	return runtime.DecoderFunc(func(v interface{}) error {
		b, err := io.ReadAll(r)
		*v.(*string) = string(b)
		return err
	})
}

// NewEncoder implements runtime.Marshaler.
func (t *TextPlain) NewEncoder(w io.Writer) runtime.Encoder {
	return runtime.EncoderFunc(func(v interface{}) error {
		_, err := w.Write([]byte(v.(string)))
		return err
	})
}

// Unmarshal implements runtime.Marshaler.
func (t *TextPlain) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
