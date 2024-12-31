package plugin_crypto

import (
	"encoding/base64"
	"fmt"
	"net/http"

	cryptopkg "m7s.live/v5/plugin/crypto/pkg"
)

func (p *CryptoPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 设置 CORS 头
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// 获取 stream 参数
	stream := r.URL.Query().Get("stream")
	if stream == "" {
		http.Error(w, "stream parameter is required", http.StatusBadRequest)
		return
	}
	//判断 stream 是否存在
	if !p.Server.Streams.Has(stream) {
		http.Error(w, "stream not found", http.StatusNotFound)
		return
	}
	keyConf, err := cryptopkg.ValidateAndCreateKey(p.IsStatic, p.Algo, p.Secret.Key, p.Secret.Iv, stream)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// cryptor, err := method.GetCryptor(p.Algo, keyConf)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }
	// w.Write([]byte(cryptor.GetKey()))

	w.Write([]byte(fmt.Sprintf("%s.%s", base64.RawStdEncoding.EncodeToString([]byte(keyConf.Key)), base64.RawStdEncoding.EncodeToString([]byte(keyConf.Iv)))))
}
