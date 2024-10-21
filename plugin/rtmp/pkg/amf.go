package rtmp

import (
	"fmt"
	"io"
	"reflect"

	"m7s.live/v5/pkg/util"
)

// Action Message Format -- AMF 0
// Action Message Format -- AMF 3
// http://download.macromedia.com/pub/labs/amf/amf0_spec_121207.pdf
// http://wwwimages.adobe.com/www.adobe.com/content/dam/Adobe/en/devnet/amf/pdf/amf-file-format-spec.pdf

// AMF Object == AMF Object Type(1 byte) + AMF Object Value
//
// AMF Object Value :
// AMF0_STRING : 2 bytes(datasize,记录string的长度) + data(string)
// AMF0_OBJECT : AMF0_STRING + AMF Object
// AMF0_NULL : 0 byte
// AMF0_NUMBER : 8 bytes
// AMF0_DATE : 10 bytes
// AMF0_BOOLEAN : 1 byte
// AMF0_ECMA_ARRAY : 4 bytes(arraysize,记录数组的长度) + AMF0_OBJECT
// AMF0_STRICT_ARRAY : 4 bytes(arraysize,记录数组的长度) + AMF Object

// 实际测试时,AMF0_ECMA_ARRAY数据如下:
// 8 0 0 0 13 0 8 100 117 114 97 116 105 111 110 0 0 0 0 0 0 0 0 0 0 5 119 105 100 116 104 0 64 158 0 0 0 0 0 0 0 6 104 101 105 103 104 116 0 64 144 224 0 0 0 0 0
// 8 0 0 0 13 | { 0 8 100 117 114 97 116 105 111 110 --- 0 0 0 0 0 0 0 0 0 } | { 0 5 119 105 100 116 104 --- 0 64 158 0 0 0 0 0 0 } | { 0 6 104 101 105 103 104 116 --- 0 64 144 224 0 0 0 0 0 } |...
// 13 | {AMF0_STRING --- AMF0_NUMBER} | {AMF0_STRING --- AMF0_NUMBER} | {AMF0_STRING --- AMF0_NUMBER} | ...
// 13 | {AMF0_OBJECT} | {AMF0_OBJECT} | {AMF0_OBJECT} | ...
// 13 | {duration --- 0} | {width --- 1920} | {height --- 1080} | ...

const (
	AMF0_NUMBER = iota // 浮点数
	AMF0_BOOLEAN
	AMF0_STRING
	AMF0_OBJECT
	AMF0_MOVIECLIP
	AMF0_NULL
	AMF0_UNDEFINED
	AMF0_REFERENCE
	AMF0_ECMA_ARRAY
	AMF0_END_OBJECT
	AMF0_STRICT_ARRAY
	AMF0_DATE
	AMF0_LONG_STRING
	AMF0_UNSUPPORTED
	AMF0_RECORDSET
	AMF0_XML_DOCUMENT
	AMF0_TYPED_OBJECT
	AMF0_AVMPLUS_OBJECT
)

var (
	END_OBJ   = []byte{0, 0, AMF0_END_OBJECT}
	ObjectEnd = &struct{}{}
	Undefined = &struct{}{}
)

type IAMF interface {
	util.IBuffer
	Unmarshal() (any, error)
	Marshal(any) []byte
	Marshals(...any) []byte
}

type EcmaArray map[string]any

type AMF struct {
	util.Buffer
}

func ReadAMF[T string | float64 | bool | map[string]any](amf *AMF) (result T) {
	value, err := amf.Unmarshal()
	if err != nil {
		return
	}
	result, _ = value.(T)
	return
}

func (amf *AMF) ReadShortString() (result string) {
	return ReadAMF[string](amf)
}

func (amf *AMF) ReadNumber() (result float64) {
	return ReadAMF[float64](amf)
}

func (amf *AMF) ReadObject() (result map[string]any) {
	return ReadAMF[map[string]any](amf)
}

func (amf *AMF) ReadBool() (result bool) {
	return ReadAMF[bool](amf)
}

func (amf *AMF) readKey() (string, error) {
	if !amf.CanReadN(2) {
		return "", io.ErrUnexpectedEOF
	}
	l := int(amf.ReadUint16())
	if !amf.CanReadN(l) {
		return "", io.ErrUnexpectedEOF
	}
	return string(amf.ReadN(l)), nil
}

func (amf *AMF) readProperty(m map[string]any) (obj map[string]any, err error) {
	var k string
	var v any
	if k, err = amf.readKey(); err == nil {
		if v, err = amf.Unmarshal(); k == "" && v == ObjectEnd {
			obj = m
		} else if err == nil {
			m[k] = v
		}
	}
	return
}

func (amf *AMF) Unmarshal() (obj any, err error) {
	if !amf.CanRead() {
		return nil, io.ErrUnexpectedEOF
	}
	defer func(b util.Buffer) {
		if err != nil {
			amf.Buffer = b
		}
	}(amf.Buffer)
	switch t := amf.ReadByte(); t {
	case AMF0_NUMBER:
		if !amf.CanReadN(8) {
			return 0, io.ErrUnexpectedEOF
		}
		obj = amf.ReadFloat64()
	case AMF0_BOOLEAN:
		if !amf.CanRead() {
			return false, io.ErrUnexpectedEOF
		}
		obj = amf.ReadByte() == 1
	case AMF0_STRING:
		obj, err = amf.readKey()
	case AMF0_OBJECT:
		var result map[string]any
		for m := make(map[string]any); err == nil && result == nil; result, err = amf.readProperty(m) {
		}
		obj = result
	case AMF0_NULL:
		return nil, nil
	case AMF0_UNDEFINED:
		return Undefined, nil
	case AMF0_ECMA_ARRAY:
		_ = amf.ReadUint32() // size
		var result map[string]any
		for m := make(map[string]any); err == nil && result == nil; result, err = amf.readProperty(m) {
		}
		obj = EcmaArray(result)
	case AMF0_END_OBJECT:
		return ObjectEnd, nil
	case AMF0_STRICT_ARRAY:
		size := amf.ReadUint32()
		var list []any
		for i := uint32(0); i < size; i++ {
			v, err := amf.Unmarshal()
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		obj = list
	case AMF0_DATE:
		if !amf.CanReadN(10) {
			return 0, io.ErrUnexpectedEOF
		}
		obj = amf.ReadFloat64()
		amf.ReadN(2)
	case AMF0_LONG_STRING,
		AMF0_XML_DOCUMENT:
		if !amf.CanReadN(4) {
			return "", io.ErrUnexpectedEOF
		}
		l := int(amf.ReadUint32())
		if !amf.CanReadN(l) {
			return "", io.ErrUnexpectedEOF
		}
		obj = string(amf.ReadN(l))
	default:
		err = fmt.Errorf("unsupported type:%d", t)
	}
	return
}

func (amf *AMF) writeProperty(key string, v any) {
	amf.WriteUint16(uint16(len(key)))
	amf.WriteString(key)
	amf.Marshal(v)
}

func MarshalAMFs(v ...any) []byte {
	var amf AMF
	return amf.Marshals(v...)
}

func (amf *AMF) Marshals(v ...any) []byte {
	for _, vv := range v {
		amf.Marshal(vv)
	}
	return amf.Buffer
}

func (amf *AMF) Marshal(v any) []byte {
	if v == nil {
		amf.WriteByte(AMF0_NULL)
		return amf.Buffer
	}
	switch vv := v.(type) {
	case string:
		if l := len(vv); l > 0xFFFF {
			amf.WriteByte(AMF0_LONG_STRING)
			amf.WriteUint32(uint32(l))
		} else {
			amf.WriteByte(AMF0_STRING)
			amf.WriteUint16(uint16(l))
		}
		amf.WriteString(vv)
	case float64, uint, float32, int, int16, int32, int64, uint16, uint32, uint64, uint8, int8:
		amf.WriteByte(AMF0_NUMBER)
		amf.WriteFloat64(ToFloat64(vv))
	case bool:
		amf.WriteByte(AMF0_BOOLEAN)
		if vv {
			amf.WriteByte(1)
		} else {
			amf.WriteByte(0)
		}
	case EcmaArray:
		if vv == nil {
			amf.WriteByte(AMF0_NULL)
			return amf.Buffer
		}
		amf.WriteByte(AMF0_ECMA_ARRAY)
		amf.WriteUint32(uint32(len(vv)))
		for k, v := range vv {
			amf.writeProperty(k, v)
		}
		amf.Write(END_OBJ)
	case map[string]any:
		if vv == nil {
			amf.WriteByte(AMF0_NULL)
			return amf.Buffer
		}
		amf.WriteByte(AMF0_OBJECT)
		for k, v := range vv {
			amf.writeProperty(k, v)
		}
		amf.Write(END_OBJ)
	default:
		v := reflect.ValueOf(vv)
		if !v.IsValid() {
			amf.WriteByte(AMF0_NULL)
			return amf.Buffer
		}
		switch v.Kind() {
		case reflect.Slice, reflect.Array:
			amf.WriteByte(AMF0_STRICT_ARRAY)
			size := v.Len()
			amf.WriteUint32(uint32(size))
			for i := 0; i < size; i++ {
				amf.Marshal(v.Index(i).Interface())
			}
		case reflect.Ptr:
			vv := reflect.Indirect(v)
			if vv.Kind() == reflect.Struct {
				amf.WriteByte(AMF0_OBJECT)
				for i := 0; i < vv.NumField(); i++ {
					amf.writeProperty(vv.Type().Field(i).Name, vv.Field(i).Interface())
				}
				amf.Write(END_OBJ)
			}
		default:
			panic("amf Marshal faild")
		}
	}
	return amf.Buffer
}

func ToFloat64(num any) float64 {
	switch v := num.(type) {
	case uint:
		return float64(v)
	case int:
		return float64(v)
	case uint8:
		return float64(v)
	case uint16:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case int8:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	}
	return 0
}
