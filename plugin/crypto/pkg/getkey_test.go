package crypto

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGetKey(t *testing.T) {
	stream := "/hdl/live/test0.flv"
	host := "http://localhost:8080/crypto/?stream="

	r, err := http.DefaultClient.Get(host + stream)
	if err != nil {
		t.Error("get", err)
		return
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Error("read", err)
		return
	}
	b64 := strings.Split(string(b), ".")

	key, err := base64.RawStdEncoding.DecodeString(b64[0])
	t.Log("key", key, err)
	iv, err := base64.RawStdEncoding.DecodeString(b64[1])
	t.Log("iv", iv, err)
}
