package config

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"
)

type Property struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Enum        []struct {
		Label string `json:"label"`
		Value any    `json:"value"`
	} `json:"enum,omitempty"`
	Items          *Object        `json:"items,omitempty"`
	Properties     map[string]any `json:"properties,omitempty"`
	Default        any            `json:"default,omitempty"`
	Decorator      string         `json:"x-decorator"`
	DecoratorProps map[string]any `json:"x-decorator-props,omitempty"`
	Component      string         `json:"x-component"`
	ComponentProps map[string]any `json:"x-component-props,omitempty"`
	Index          int            `json:"x-index"`
}

type Card struct {
	Type           string         `json:"type"`
	Properties     map[string]any `json:"properties,omitempty"`
	Component      string         `json:"x-component"`
	ComponentProps map[string]any `json:"x-component-props,omitempty"`
	Index          int            `json:"x-index"`
}

type Object struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

func (config *Config) schema(index int) (r any) {
	defer func() {
		err := recover()
		if err != nil {
			slog.Error(err.(error).Error())
		}
	}()
	if config.props != nil {
		r := Card{
			Type:       "void",
			Component:  "Card",
			Properties: make(map[string]any),
			Index:      index,
		}
		r.ComponentProps = map[string]any{
			"title": config.name,
		}
		for i, v := range config.props {
			if strings.HasPrefix(v.tag.Get("desc"), "废弃") {
				continue
			}
			r.Properties[v.name] = v.schema(i)
		}
		return r
	} else {
		p := Property{
			Title:   config.name,
			Default: config.GetValue(),
			DecoratorProps: map[string]any{
				"tooltip": config.tag.Get("desc"),
			},
			ComponentProps: map[string]any{},
			Decorator:      "FormItem",
			Index:          index,
		}
		if config.Modify != nil {
			p.Description = "已动态修改"
		} else if config.Env != nil {
			p.Description = "使用环境变量中的值"
		} else if config.File != nil {
			p.Description = "使用配置文件中的值"
		} else if config.Global != nil {
			p.Description = "已使用全局配置中的值"
		}
		p.Enum = config.Enum
		switch config.Ptr.Type() {
		case regexpType:
			p.Type = "string"
			p.Component = "Input"
			p.DecoratorProps["addonAfter"] = "正则表达式"
			str := config.GetValue().(Regexp).String()
			p.ComponentProps = map[string]any{
				"placeholder": str,
			}
			p.Default = str
		case durationType:
			p.Type = "string"
			p.Component = "Input"
			str := config.GetValue().(time.Duration).String()
			p.ComponentProps = map[string]any{
				"placeholder": str,
			}
			p.Default = str
			p.DecoratorProps["addonAfter"] = "时间,单位：s,m,h,d，例如：100ms, 10s, 4m, 1h"
		default:
			switch config.Ptr.Kind() {
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64:
				p.Type = "number"
				p.Component = "InputNumber"
				p.ComponentProps = map[string]any{
					"placeholder": fmt.Sprintf("%v", config.GetValue()),
				}
			case reflect.Bool:
				p.Type = "boolean"
				p.Component = "Switch"
			case reflect.String:
				p.Type = "string"
				p.Component = "Input"
				p.ComponentProps = map[string]any{
					"placeholder": config.GetValue(),
				}
			case reflect.Slice:
				p.Type = "array"
				p.Component = "Input"
				p.ComponentProps = map[string]any{
					"placeholder": config.GetValue(),
				}
				p.DecoratorProps["addonAfter"] = "数组，每个元素用逗号分隔"
			case reflect.Map:
				var children []struct {
					Key   string `json:"mkey"`
					Value any    `json:"mvalue"`
				}
				p := Property{
					Type:      "array",
					Component: "ArrayTable",
					Decorator: "FormItem",
					Properties: map[string]any{
						"addition": map[string]string{
							"type":        "void",
							"title":       "添加",
							"x-component": "ArrayTable.Addition",
						},
					},
					Index: index,
					Title: config.name,
					Items: &Object{
						Type: "object",
						Properties: map[string]any{
							"c1": Card{
								Type:      "void",
								Component: "ArrayTable.Column",
								ComponentProps: map[string]any{
									"title": config.tag.Get("key"),
									"width": 300,
								},
								Properties: map[string]any{
									"mkey": Property{
										Type:      "string",
										Decorator: "FormItem",
										Component: "Input",
									},
								},
								Index: 0,
							},
							"c2": Card{
								Type:      "void",
								Component: "ArrayTable.Column",
								ComponentProps: map[string]any{
									"title": config.tag.Get("value"),
								},
								Properties: map[string]any{
									"mvalue": Property{
										Type:      "string",
										Decorator: "FormItem",
										Component: "Input",
									},
								},
								Index: 1,
							},
							"operator": Card{
								Type:      "void",
								Component: "ArrayTable.Column",
								ComponentProps: map[string]any{
									"title": "操作",
								},
								Properties: map[string]any{
									"remove": Card{
										Type:      "void",
										Component: "ArrayTable.Remove",
									},
								},
								Index: 2,
							},
						},
					},
				}
				iter := config.Ptr.MapRange()
				for iter.Next() {
					children = append(children, struct {
						Key   string `json:"mkey"`
						Value any    `json:"mvalue"`
					}{
						Key:   iter.Key().String(),
						Value: iter.Value().Interface(),
					})
				}
				p.Default = children
				return p
			default:

			}
		}
		if len(p.Enum) > 0 {
			p.Component = "Radio.Group"
		}
		return p
	}
}

func (config *Config) GetFormily() (r Object) {
	var fromItems = make(map[string]any)
	r.Type = "object"
	r.Properties = map[string]any{
		"layout": Card{
			Type:      "void",
			Component: "FormLayout",
			ComponentProps: map[string]any{
				"labelCol":   4,
				"wrapperCol": 20,
			},
			Properties: fromItems,
		},
	}
	for i, v := range config.props {
		if strings.HasPrefix(v.tag.Get("desc"), "废弃") {
			continue
		}
		fromItems[v.name] = v.schema(i)
	}
	return
}
