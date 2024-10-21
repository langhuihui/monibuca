package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"log/slog"
	"m7s.live/v5/pkg/task"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

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
	grpcMux       *runtime.ServeMux
	middlewares   []Middleware
}
type HTTPConfig interface {
	GetHTTPConfig() *HTTP
	// Handle(string, http.Handler)
	// Handler(*http.Request) (http.Handler, string)
	// AddMiddleware(Middleware)
}

func (config *HTTP) GetHandler() http.Handler {
	if config.grpcMux != nil {
		return config.grpcMux
	}
	return config.mux
}

func (config *HTTP) GetHttpMux() *http.ServeMux {
	return config.mux
}

func (config *HTTP) GetGRPCMux() *runtime.ServeMux {
	return config.grpcMux
}

func (config *HTTP) SetMux(mux *runtime.ServeMux) {
	config.grpcMux = mux
	handler := func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
		config.mux.ServeHTTP(w, r)
	}
	mux.HandlePath("GET", "/", handler)
	mux.HandlePath("POST", "/", handler)
}

func (config *HTTP) AddMiddleware(middleware Middleware) {
	config.middlewares = append(config.middlewares, middleware)
}

func (config *HTTP) Handle(path string, f http.Handler, last bool) {
	if config.mux == nil {
		config.mux = http.NewServeMux()
	}
	if config.CORS {
		f = CORS(f)
	}
	if config.UserName != "" && config.Password != "" {
		f = BasicAuth(config.UserName, config.Password, f)
	}
	for _, middleware := range config.middlewares {
		f = middleware(path, f)
	}
	config.mux.Handle(path, f)
}

func (config *HTTP) GetHTTPConfig() *HTTP {
	return config
}

// func (config *HTTP) Handler(r *http.Request) (h http.Handler, pattern string) {
// 	return config.mux.Handler(r)
// }

func (config *HTTP) CreateHTTPWork(logger *slog.Logger) *ListenHTTPWork {
	ret := &ListenHTTPWork{HTTP: config}
	ret.Logger = logger.With("addr", config.ListenAddr)
	return ret
}

func (config *HTTP) CreateHTTPSWork(logger *slog.Logger) *ListenHTTPSWork {
	ret := &ListenHTTPSWork{ListenHTTPWork{HTTP: config}}
	ret.Logger = logger.With("addr", config.ListenAddrTLS)
	return ret
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Access-Control-Allow-Credentials", "true")
		header.Set("Cross-Origin-Resource-Policy", "cross-origin")
		header.Set("Access-Control-Allow-Headers", "Content-Type,Access-Token")
		header.Set("Access-Control-Allow-Private-Network", "true")
		origin := r.Header["Origin"]
		if len(origin) == 0 {
			header.Set("Access-Control-Allow-Origin", "*")
		} else {
			header.Set("Access-Control-Allow-Origin", origin[0])
		}
		if next != nil && r.Method != "OPTIONS" {
			next.ServeHTTP(w, r)
		}
	})
}

func BasicAuth(u, p string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the username and password from the request
		// Authorization header. If no Authentication header is present
		// or the header value is invalid, then the 'ok' return value
		// will be false.
		username, password, ok := r.BasicAuth()
		if ok {
			// Calculate SHA-256 hashes for the provided and expected
			// usernames and passwords.
			usernameHash := sha256.Sum256([]byte(username))
			passwordHash := sha256.Sum256([]byte(password))
			expectedUsernameHash := sha256.Sum256([]byte(u))
			expectedPasswordHash := sha256.Sum256([]byte(p))

			// 使用 subtle.ConstantTimeCompare() 进行校验
			// the provided username and password hashes equal the
			// expected username and password hashes. ConstantTimeCompare
			// 如果值相等，则返回1，否则返回0。
			// Importantly, we should to do the work to evaluate both the
			// username and password before checking the return values to
			// 避免泄露信息。
			usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1)
			passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1)

			// If the username and password are correct, then call
			// the next handler in the chain. Make sure to return
			// afterwards, so that none of the code below is run.
			if usernameMatch && passwordMatch {
				if next != nil {
					next.ServeHTTP(w, r)
				}
				return
			}
		}

		// If the Authentication header is not present, is invalid, or the
		// username or password is wrong, then set a WWW-Authenticate
		// header to inform the client that we expect them to use basic
		// authentication and send a 401 Unauthorized response.
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

type ListenHTTPWork struct {
	task.Task
	*HTTP
	*http.Server
}

func (task *ListenHTTPWork) Start() (err error) {
	task.Server = &http.Server{
		Addr:         task.ListenAddr,
		ReadTimeout:  task.HTTP.ReadTimeout,
		WriteTimeout: task.HTTP.WriteTimeout,
		IdleTimeout:  task.HTTP.IdleTimeout,
		Handler:      task.GetHandler(),
	}
	return
}

func (task *ListenHTTPWork) Go() error {
	task.Info("listen http")
	return task.Server.ListenAndServe()
}

func (task *ListenHTTPWork) Dispose() {
	task.Info("http server stop")
	task.Server.Close()
}

type ListenHTTPSWork struct {
	ListenHTTPWork
}

func (task *ListenHTTPSWork) Start() (err error) {
	cer, _ := tls.X509KeyPair(LocalCert, LocalKey)
	task.Server = &http.Server{
		Addr:         task.HTTP.ListenAddrTLS,
		ReadTimeout:  task.HTTP.ReadTimeout,
		WriteTimeout: task.HTTP.WriteTimeout,
		IdleTimeout:  task.HTTP.IdleTimeout,
		Handler:      task.HTTP.GetHandler(),
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
	return
}

func (task *ListenHTTPSWork) Go() error {
	task.Info("listen https")
	return task.Server.ListenAndServeTLS(task.HTTP.CertFile, task.HTTP.KeyFile)
}
