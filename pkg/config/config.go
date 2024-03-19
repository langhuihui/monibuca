package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Ptr     reflect.Value //指向配置结构体值,优先级：动态修改值>环境变量>配置文件>defaultYaml>全局配置>默认值
	Modify  any           //动态修改的值
	Env     any           //环境变量中的值
	File    any           //配置文件中的值
	Global  *Config       //全局配置中的值,指针类型
	Default any           //默认值
	Enum    []struct {
		Label string `json:"label"`
		Value any    `json:"value"`
	}
	name     string // 小写
	propsMap map[string]*Config
	props    []*Config
	tag      reflect.StructTag
}

var durationType = reflect.TypeOf(time.Duration(0))
var regexpType = reflect.TypeOf(Regexp{})

type Plugin interface {
	// 可能的入参类型：FirstConfig 第一次初始化配置，Config 后续配置更新，SE系列（StateEvent）流状态变化事件
	OnEvent(any)
}

type TCPPlugin interface {
	Plugin
	ServeTCP(net.Conn)
}

type HTTPPlugin interface {
	Plugin
	http.Handler
}

type QuicPlugin interface {
	Plugin
	ServeQuic(quic.Connection)
}

func (config *Config) Range(f func(key string, value Config)) {
	if m, ok := config.GetValue().(map[string]Config); ok {
		for k, v := range m {
			f(k, v)
		}
	}
}

func (config *Config) IsMap() bool {
	_, ok := config.GetValue().(map[string]Config)
	return ok
}

func (config *Config) Get(key string) (v *Config) {
	if config.propsMap == nil {
		config.propsMap = make(map[string]*Config)
	}
	if v, ok := config.propsMap[key]; ok {
		return v
	} else {
		v = &Config{
			name: key,
		}
		config.propsMap[key] = v
		config.props = append(config.props, v)
		return v
	}
}

func (config Config) Has(key string) (ok bool) {
	if config.propsMap == nil {
		return false
	}
	_, ok = config.propsMap[strings.ToLower(key)]
	return ok
}

func (config *Config) MarshalJSON() ([]byte, error) {
	if config.propsMap == nil {
		return json.Marshal(config.GetValue())
	}
	return json.Marshal(config.propsMap)
}

func (config *Config) GetValue() any {
	return config.Ptr.Interface()
}

// Parse 第一步读取配置结构体的默认值
func (config *Config) Parse(s any, prefix ...string) {
	var t reflect.Type
	var v reflect.Value
	if vv, ok := s.(reflect.Value); ok {
		t, v = vv.Type(), vv
	} else {
		t, v = reflect.TypeOf(s), reflect.ValueOf(s)
	}
	if t.Kind() == reflect.Pointer {
		t, v = t.Elem(), v.Elem()
	}
	config.Ptr = v
	config.Default = v.Interface()
	if len(prefix) > 0 { // 读取环境变量
		envKey := strings.Join(prefix, "_")
		if envValue := os.Getenv(envKey); envValue != "" {
			envv := config.assign(strings.ToLower(prefix[0]), envValue)
			config.Env = envv.Interface()
			config.Ptr.Set(envv)
		}
	}
	if t.Kind() == reflect.Struct && t != regexpType {
		for i, j := 0, t.NumField(); i < j; i++ {
			ft, fv := t.Field(i), v.Field(i)
			if !ft.IsExported() {
				continue
			}
			name := strings.ToLower(ft.Name)
			if tag := ft.Tag.Get("yaml"); tag != "" {
				if tag == "-" {
					continue
				}
				name, _, _ = strings.Cut(tag, ",")
			}
			prop := config.Get(name)
			prop.Parse(fv, append(prefix, strings.ToUpper(ft.Name))...)
			prop.tag = ft.Tag
			for _, kv := range strings.Split(ft.Tag.Get("enum"), ",") {
				kvs := strings.Split(kv, ":")
				if len(kvs) != 2 {
					continue
				}
				var tmp struct {
					Value any
				}
				yaml.Unmarshal([]byte(fmt.Sprintf("value: %s", strings.TrimSpace(kvs[0]))), &tmp)
				prop.Enum = append(prop.Enum, struct {
					Label string `json:"label"`
					Value any    `json:"value"`
				}{
					Label: strings.TrimSpace(kvs[1]),
					Value: tmp.Value,
				})
			}
		}
	}
}

// ParseDefaultYaml 第二步读取全局配置
func (config *Config) ParseGlobal(g *Config) {
	config.Global = g
	if config.propsMap != nil {
		for k, v := range config.propsMap {
			v.ParseGlobal(g.Get(k))
		}
	} else {
		config.Ptr.Set(g.Ptr)
	}
}

// ParseDefaultYaml 第三步读取内嵌默认配置
func (config *Config) ParseDefaultYaml(defaultYaml map[string]any) {
	if defaultYaml == nil {
		return
	}
	for k, v := range defaultYaml {
		if config.Has(k) {
			if prop := config.Get(k); prop.props != nil {
				if v != nil {
					prop.ParseDefaultYaml(v.(map[string]any))
				}
			} else {
				dv := prop.assign(k, v)
				prop.Default = dv.Interface()
				if prop.Env == nil {
					prop.Ptr.Set(dv)
				}
			}
		}
	}
}

// ParseFile 第四步读取用户配置文件
func (config *Config) ParseUserFile(conf map[string]any) {
	if conf == nil {
		return
	}
	config.File = conf
	for k, v := range conf {
		if config.Has(k) {
			if prop := config.Get(k); prop.props != nil {
				if v != nil {
					prop.ParseUserFile(v.(map[string]any))
				}
			} else {
				fv := prop.assign(k, v)
				prop.File = fv.Interface()
				if prop.Env == nil {
					prop.Ptr.Set(fv)
				}
			}
		}
	}
}

// ParseModifyFile 第五步读取动态修改配置文件
func (config *Config) ParseModifyFile(conf map[string]any) {
	if conf == nil {
		return
	}
	config.Modify = conf
	for k, v := range conf {
		if config.Has(k) {
			if prop := config.Get(k); prop.props != nil {
				if v != nil {
					vmap := v.(map[string]any)
					prop.ParseModifyFile(vmap)
					if len(vmap) == 0 {
						delete(conf, k)
					}
				}
			} else {
				mv := prop.assign(k, v)
				v = mv.Interface()
				vwm := prop.valueWithoutModify()
				if equal(vwm, v) {
					delete(conf, k)
					if prop.Modify != nil {
						prop.Modify = nil
						prop.Ptr.Set(reflect.ValueOf(vwm))
					}
					continue
				}
				prop.Modify = v
				prop.Ptr.Set(mv)
			}
		}
	}
	if len(conf) == 0 {
		config.Modify = nil
	}
}

func (config *Config) valueWithoutModify() any {
	if config.Env != nil {
		return config.Env
	}
	if config.File != nil {
		return config.File
	}
	if config.Global != nil {
		return config.Global.GetValue()
	}
	return config.Default
}

func equal(vwm, v any) bool {
	ft := reflect.TypeOf(vwm)
	switch ft {
	case regexpType:
		return vwm.(Regexp).String() == v.(Regexp).String()
	default:
		switch ft.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return reflect.DeepEqual(vwm, v)
		}
		return vwm == v
	}
}

func (config *Config) GetMap() map[string]any {
	m := make(map[string]any)
	for k, v := range config.propsMap {
		if v.props != nil {
			if vv := v.GetMap(); vv != nil {
				m[k] = vv
			}
		} else if v.GetValue() != nil {
			m[k] = v.GetValue()
		}
	}
	if len(m) > 0 {
		return m
	}
	return nil
}

var regexPureNumber = regexp.MustCompile(`^\d+$`)

func (config *Config) assign(k string, v any) (target reflect.Value) {
	ft := config.Ptr.Type()

	source := reflect.ValueOf(v)

	switch ft {
	case durationType:
		target = reflect.New(ft).Elem()
		if source.Type() == durationType {
			target.Set(source)
		} else if source.IsZero() || !source.IsValid() {
			target.SetInt(0)
		} else {
			timeStr := source.String()
			if d, err := time.ParseDuration(timeStr); err == nil && !regexPureNumber.MatchString(timeStr) {
				target.SetInt(int64(d))
			} else {
				slog.Error("invalid duration value please add unit (s,m,h,d)，eg: 100ms, 10s, 4m, 1h", "key", k, "value", source)
				os.Exit(1)
			}
		}
	case regexpType:
		target = reflect.New(ft).Elem()
		regexpStr := source.String()
		target.Set(reflect.ValueOf(Regexp{regexp.MustCompile(regexpStr)}))
	default:
		tmpStruct := reflect.StructOf([]reflect.StructField{
			{
				Name: strings.ToUpper(k),
				Type: ft,
			},
		})
		tmpValue := reflect.New(tmpStruct)
		tmpByte, _ := yaml.Marshal(map[string]any{k: v})
		yaml.Unmarshal(tmpByte, tmpValue.Interface())
		target = tmpValue.Elem().Field(0)
	}
	return
}
