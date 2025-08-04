package plugin_crypto

import (
	"encoding/base64"
	"fmt"
	"net/http"
)

func (p *CryptoPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 获取 stream 参数
	stream := r.URL.Query().Get("stream")
	if stream == "" {
		http.Error(w, "stream parameter is required", http.StatusBadRequest)
		return
	}
	//判断 stream 是否存在
	if pub, ok := p.Server.Streams.Get(stream); !ok {
		http.Error(w, "stream not found", http.StatusNotFound)
		return
	} else {
		key, ok := pub.GetDescription("key")
		if !ok {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		iv, ok := pub.GetDescription("iv")
		if !ok {
			http.Error(w, "iv not found", http.StatusNotFound)
			return
		}
		w.Write([]byte(fmt.Sprintf("%s.%s", base64.RawStdEncoding.EncodeToString([]byte(key.(string))), base64.RawStdEncoding.EncodeToString([]byte(iv.(string))))))
	}
}
