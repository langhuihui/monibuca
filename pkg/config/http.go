package config

import (
	"crypto/tls"
	"net/http"
	"time"
)

var _ HTTPConfig = (*HTTP)(nil)

type Middleware func(string, http.Handler) http.Handler
type HTTP struct {
	ListenAddr    string        `desc:"监听地址"`
	ListenAddrTLS string        `desc:"监听地址HTTPS"`
	CertFile      string        `desc:"HTTPS证书文件"`
	KeyFile       string        `desc:"HTTPS密钥文件"`
	CORS          bool          `default:"true" desc:"是否自动添加CORS头"` //是否自动添加CORS头
	UserName      string        `desc:"基本身份认证用户名"`
	Password      string        `desc:"基本身份认证密码"`
	ReadTimeout   time.Duration `desc:"读取超时"`
	WriteTimeout  time.Duration `desc:"写入超时"`
	IdleTimeout   time.Duration `desc:"空闲超时"`
	mux           *http.ServeMux
	server        *http.Server
	serverTLS     *http.Server
	middlewares   []Middleware
}
type HTTPConfig interface {
	GetHTTPConfig() *HTTP
	Handle(string, http.Handler)
	Handler(*http.Request) (http.Handler, string)
	AddMiddleware(Middleware)
}

func (config *HTTP) AddMiddleware(middleware Middleware) {
	config.middlewares = append(config.middlewares, middleware)
}

func (config *HTTP) Handle(path string, f http.Handler) {
	if config.mux == nil {
		config.mux = http.NewServeMux()
	}
	if config.CORS {
		// f = util.CORS(f)
	}
	if config.UserName != "" && config.Password != "" {
		// f = util.BasicAuth(config.UserName, config.Password, f)
	}
	for _, middleware := range config.middlewares {
		f = middleware(path, f)
	}
	config.mux.Handle(path, f)
}

func (config *HTTP) GetHTTPConfig() *HTTP {
	return config
}

func (config *HTTP) Handler(r *http.Request) (h http.Handler, pattern string) {
	return config.mux.Handler(r)
}

func (config *HTTP) StopListen() {
	if config.server != nil {
		config.server.Close()
	}
	if config.serverTLS != nil {
		config.serverTLS.Close()
	}
}
func (config *HTTP) ListenTLS() error {
	cer, _ := tls.X509KeyPair(LocalCert, LocalKey)
	config.serverTLS = &http.Server{
		Addr:         config.ListenAddrTLS,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
		Handler:      config.mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cer},
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
				//tls.TLS_RSA_WITH_AES_128_CBC_SHA,
				//tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				//tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
				//tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
			},
		},
	}
	return config.serverTLS.ListenAndServeTLS(config.CertFile, config.KeyFile)
}

func (config *HTTP) Listen() error {
	config.server = &http.Server{
		Addr:         config.ListenAddr,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
		Handler:      config.mux,
	}
	return config.server.ListenAndServe()
}
