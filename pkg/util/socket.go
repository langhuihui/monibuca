package util

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

func FetchValue[T any](t T) func() T {
	return func() T {
		return t
	}
}

const (
	APIErrorNone   = 0
	APIErrorDecode = iota + 4000
	APIErrorQueryParse
	APIErrorNoBody
)

const (
	APIErrorNotFound = iota + 4040
	APIErrorNoStream
	APIErrorNoConfig
	APIErrorNoPusher
	APIErrorNoSubscriber
	APIErrorNoSEI
)

const (
	APIErrorInternal = iota + 5000
	APIErrorJSONEncode
	APIErrorPublish
	APIErrorSave
	APIErrorOpen
)

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
}

type APIResult struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"msg"`
}

func ReturnValue(v any, rw http.ResponseWriter, r *http.Request) {
	ReturnFetchValue(FetchValue(v), rw, r)
}

func ReturnOK(rw http.ResponseWriter, r *http.Request) {
	ReturnError(0, "ok", rw, r)
}

func ReturnError(code int, msg string, rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	isJson := query.Get("format") == "json"
	if isJson {
		if err := json.NewEncoder(rw).Encode(APIError{code, msg}); err != nil {
			json.NewEncoder(rw).Encode(APIError{
				Code:    APIErrorJSONEncode,
				Message: err.Error(),
			})
		}
	} else {
		switch true {
		case code == 0:
			http.Error(rw, msg, http.StatusOK)
		case code/10 == 404:
			http.Error(rw, msg, http.StatusNotFound)
		case code > 5000:
			http.Error(rw, msg, http.StatusInternalServerError)
		default:
			http.Error(rw, msg, http.StatusBadRequest)
		}
	}
}
func ReturnFetchList[T any](fetch func() []T, rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	isYaml := query.Get("format") == "yaml"
	isJson := query.Get("format") == "json"
	pageSize := query.Get("pageSize")
	pageNum := query.Get("pageNum")
	data := fetch()
	var output any
	output = data
	if pageSize != "" && pageNum != "" {
		pageSizeInt, _ := strconv.Atoi(pageSize)
		pageNumInt, _ := strconv.Atoi(pageNum)
		if pageSizeInt > 0 && pageNumInt > 0 {
			start := (pageNumInt - 1) * pageSizeInt
			end := pageNumInt * pageSizeInt
			if start > len(data) {
				start = len(data)
			}
			if end > len(data) {
				end = len(data)
			}
			output = map[string]any{
				"total":    len(data),
				"list":     data[start:end],
				"pageSize": pageSizeInt,
				"pageNum":  pageNumInt,
			}
		}
	}
	rw.Header().Set("Content-Type", Conditoinal(isYaml, "text/yaml", "application/json"))
	if isYaml {
		if err := yaml.NewEncoder(rw).Encode(output); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
	} else if isJson {
		if err := json.NewEncoder(rw).Encode(APIResult{
			Code:    0,
			Data:    output,
			Message: "ok",
		}); err != nil {
			json.NewEncoder(rw).Encode(APIError{
				Code:    APIErrorJSONEncode,
				Message: err.Error(),
			})
		}
	} else {
		if err := json.NewEncoder(rw).Encode(output); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
	}
}
func ReturnFetchValue[T any](fetch func() T, rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	isYaml := query.Get("format") == "yaml"
	isJson := query.Get("format") == "json"
	tickDur, err := time.ParseDuration(query.Get("interval"))
	if err != nil {
		tickDur = time.Second
	}
	if r.Header.Get("Accept") == "text/event-stream" {
		sse := NewSSE(rw, r.Context())
		tick := time.NewTicker(tickDur)
		defer tick.Stop()
		writer := Conditoinal(isYaml, sse.WriteYAML, sse.WriteJSON)
		writer(fetch())
		for range tick.C {
			if writer(fetch()) != nil {
				return
			}
		}
	} else {
		data := fetch()
		rw.Header().Set("Content-Type", Conditoinal(isYaml, "text/yaml", "application/json"))
		if isYaml {
			if err := yaml.NewEncoder(rw).Encode(data); err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
			}
		} else if isJson {
			if err := json.NewEncoder(rw).Encode(APIResult{
				Code:    0,
				Data:    data,
				Message: "ok",
			}); err != nil {
				json.NewEncoder(rw).Encode(APIError{
					Code:    APIErrorJSONEncode,
					Message: err.Error(),
				})
			}
		} else {
			t := reflect.TypeOf(data)
			switch t.Kind() {
			case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
				rw.Header().Set("Content-Type", "text/plain")
				fmt.Fprint(rw, data)
			default:
				if err := json.NewEncoder(rw).Encode(data); err != nil {
					http.Error(rw, err.Error(), http.StatusInternalServerError)
				}
			}
		}
	}
}

func ListenUDP(address string, networkBuffer int) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	if err = conn.SetReadBuffer(networkBuffer); err != nil {
		return nil, err
	}
	if err = conn.SetWriteBuffer(networkBuffer); err != nil {
		return nil, err
	}
	return conn, err
}

// CORS 加入跨域策略头包含CORP
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Access-Control-Allow-Credentials", "true")
		header.Set("Cross-Origin-Resource-Policy", "cross-origin")
		header.Set("Access-Control-Allow-Headers", "Content-Type,Access-Token")
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
