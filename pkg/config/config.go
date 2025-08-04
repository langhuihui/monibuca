package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/mcuadros/go-defaults"

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

var (
	durationType = reflect.TypeOf(time.Duration(0))
	regexpType   = reflect.TypeOf(Regexp{})
	basicTypes   = []reflect.Kind{
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.String,
	}
)

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
	key = strings.ToLower(key)
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

func (config *Config) Has(key string) (ok bool) {
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
	isStruct := t.Kind() == reflect.Struct && t != regexpType
	if isStruct {
		defaults.SetDefaults(v.Addr().Interface())
	}
	config.Ptr = v
	if !v.IsValid() {
		fmt.Println("parse to ", prefix, config.name, s, "is not valid")
		return
	}
	if l := len(prefix); l > 0 { // 读取环境变量
		name := strings.ToLower(prefix[l-1])
		_, isUnmarshaler := v.Addr().Interface().(yaml.Unmarshaler)
		tag := config.tag.Get("default")
		if tag != "" && isUnmarshaler {
			v.Set(config.assign(name, tag))
		}
		if envValue := os.Getenv(strings.Join(prefix, "_")); envValue != "" {
			v.Set(config.assign(name, envValue))
			config.Env = v.Interface()
		}
	}
	config.Default = v.Interface()
	if isStruct {
		for i, j := 0, t.NumField(); i < j; i++ {
			ft, fv := t.Field(i), v.Field(i)

			if !ft.IsExported() {
				continue
			}
			name := strings.ToLower(ft.Name)
			if name == "plugin" {
				continue
			}
			if tag := ft.Tag.Get("yaml"); tag != "" {
				if tag == "-" {
					continue
				}
				name, _, _ = strings.Cut(tag, ",")
			}
			prop := config.Get(name)

			prop.tag = ft.Tag
			if len(prefix) > 0 {
				prop.Parse(fv, append(prefix, strings.ToUpper(ft.Name))...)
			} else {
				prop.Parse(fv)
			}
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
		k = strings.ReplaceAll(k, "-", "")
		k = strings.ReplaceAll(k, "_", "")
		k = strings.ToLower(k)
		if config.Has(k) {
			if prop := config.Get(k); prop.props != nil {
				if v != nil {
					switch vv := v.(type) {
					case map[string]any:
						prop.ParseUserFile(vv)
					default:
						prop.props[0].Ptr.Set(reflect.ValueOf(v))
					}
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
	switch ft := reflect.TypeOf(vwm); ft {
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

func unmarshal(ft reflect.Type, v any) (target reflect.Value) {
	source := reflect.ValueOf(v)
	for _, t := range basicTypes {
		if source.Kind() == t && ft.Kind() == t {
			return source
		}
	}
	switch ft {
	case durationType:
		target = reflect.New(ft).Elem()
		if source.Type() == durationType {
			return source
		} else if source.IsZero() || !source.IsValid() {
			target.SetInt(0)
		} else {
			timeStr := source.String()
			if d, err := time.ParseDuration(timeStr); err == nil && !regexPureNumber.MatchString(timeStr) {
				target.SetInt(int64(d))
			} else {
				slog.Error("invalid duration value please add unit (s,m,h,d)，eg: 100ms, 10s, 4m, 1h", "value", timeStr)
				os.Exit(1)
			}
		}
	case regexpType:
		target = reflect.New(ft).Elem()
		regexpStr := source.String()
		target.Set(reflect.ValueOf(Regexp{regexp.MustCompile(regexpStr)}))
	default:
		switch ft.Kind() {
		case reflect.Struct:
			newStruct := reflect.New(ft)
			defaults.SetDefaults(newStruct.Interface())
			if value, ok := v.(map[string]any); ok {
				for i := 0; i < ft.NumField(); i++ {
					key := strings.ToLower(ft.Field(i).Name)
					if vv, ok := value[key]; ok {
						newStruct.Elem().Field(i).Set(unmarshal(ft.Field(i).Type, vv))
					}
				}
			} else {
				newStruct.Elem().Field(0).Set(unmarshal(ft.Field(0).Type, v))
			}
			return newStruct.Elem()
		case reflect.Map:
			if v != nil {
				target = reflect.MakeMap(ft)
				for k, v := range v.(map[string]any) {
					target.SetMapIndex(unmarshal(ft.Key(), k), unmarshal(ft.Elem(), v))
				}
			}
		case reflect.Slice:
			if v != nil {
				s := v.([]any)
				target = reflect.MakeSlice(ft, len(s), len(s))
				for i, v := range s {
					target.Index(i).Set(unmarshal(ft.Elem(), v))
				}
			}
		default:
			if v != nil {
				var out []byte
				var err error
				if vv, ok := v.(string); ok {
					out = []byte(fmt.Sprintf("%s: %s", "value", vv))
				} else {
					out, err = yaml.Marshal(map[string]any{"value": v})
					if err != nil {
						panic(err)
					}
				}
				tmpValue := reflect.New(reflect.StructOf([]reflect.StructField{
					{
						Name: "Value",
						Type: ft,
					},
				}))
				err = yaml.Unmarshal(out, tmpValue.Interface())
				if err != nil {
					panic(err)
				}
				return tmpValue.Elem().Field(0)
			}
		}
	}
	return
}

func (config *Config) assign(k string, v any) reflect.Value {
	return unmarshal(config.Ptr.Type(), v)
}

func Parse(target any, conf map[string]any) {
	var c Config
	c.Parse(target)
	c.ParseModifyFile(maps.Clone(conf))
}
