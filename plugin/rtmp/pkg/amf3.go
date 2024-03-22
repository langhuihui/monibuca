package pkg

import (
	"errors"
	"reflect"
	"strconv"
	"unicode"
)

const (
	AMF3_UNDEFINED = iota
	AMF3_NULL
	AMF3_FALSE
	AMF3_TRUE
	AMF3_INTEGER
	AMF3_DOUBLE
	AMF3_STRING
	AMF3_XML_DOC
	AMF3_DATE
	AMF3_ARRAY
	AMF3_OBJECT
	AMF3_XML
	AMF3_BYTE_ARRAY
	AMF3_VECTOR_INT
	AMF3_VECTOR_UINT
	AMF3_VECTOR_DOUBLE
	AMF3_VECTOR_OBJECT
	AMF3_DICTIONARY
)

type AMF3 struct {
	AMF
	scEnc        map[string]int
	scDec        []string
	ocEnc        map[uintptr]int
	ocDec        []any
	reservStruct bool
}

func (amf *AMF3) readString() (string, error) {
	index, err := amf.readU29()
	if err != nil {
		return "", err
	}
	ret := ""
	if (index & 0x01) == 0 {
		ret = amf.scDec[int(index>>1)]
	} else {
		index >>= 1
		ret = string(amf.ReadN(int(index)))
	}
	if ret != "" {
		amf.scDec = append(amf.scDec, ret)
	}
	return ret, nil
}
func (amf *AMF3) Unmarshal() (obj any, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.New("amf3 unmarshal error")
		}
	}()
	switch amf.ReadByte() {
	case AMF3_NULL:
		return nil, nil
	case AMF3_FALSE:
		return false, nil
	case AMF3_TRUE:
		return true, nil
	case AMF3_INTEGER:
		return amf.readU29()
	case AMF3_DOUBLE:
		return amf.ReadFloat64(), nil
	case AMF3_STRING:
		return amf.readString()
	case AMF3_OBJECT:
		index, err := amf.readU29()
		if err != nil {
			return nil, err
		}
		if (index & 0x01) == 0 {
			return amf.ocDec[int(index>>1)], nil
		}
		if index != 0x0b {
			return nil, errors.New("invalid object type")
		}
		if amf.ReadByte() != 0x01 {
			return nil, errors.New("type object not allowed")
		}
		ret := make(map[string]any)
		amf.ocDec = append(amf.ocDec, ret)
		for {
			key, err := amf.readString()
			if err != nil {
				return nil, err
			}
			if key == "" {
				break
			}
			ret[key], err = amf.Unmarshal()
			if err != nil {
				return nil, err
			}
		}
		return ret, nil
	}
	return nil, errors.New("amf3 unmarshal error")
}

func (amf *AMF3) writeString(s string) error {
	index, ok := amf.scEnc[s]
	if ok {
		amf.writeU29(uint32(index << 1))
		return nil
	}

	err := amf.writeU29(uint32((len(s) << 1) | 0x01))
	if err != nil {
		return err
	}

	if s != "" {
		amf.scEnc[s] = len(amf.scEnc)
	}
	amf.WriteString(s)
	return nil
}

func (amf *AMF3) readU29() (uint32, error) {
	var ret uint32 = 0
	for i := 0; i < 4; i++ {
		b := amf.ReadByte()
		if i != 3 {
			ret = (ret << 7) | uint32(b&0x7f)
			if (b & 0x80) == 0 {
				break
			}
		} else {
			ret = (ret << 8) | uint32(b)
		}
	}

	return ret, nil
}
func (amf *AMF3) writeU29(value uint32) error {
	switch {
	case value < 0x80:
		amf.WriteByte(byte(value))
	case value < 0x4000:
		amf.Write([]byte{byte((value >> 7) | 0x80), byte(value & 0x7f)})
	case value < 0x200000:
		amf.Write([]byte{byte((value >> 14) | 0x80), byte((value >> 7) | 0x80), byte(value & 0x7f)})
	case value < 0x20000000:
		amf.Write([]byte{byte((value >> 22) | 0x80), byte((value >> 15) | 0x80), byte((value >> 7) | 0x80), byte(value & 0xff)})
	default:
		return errors.New("u29 over flow")
	}
	return nil
}

func (amf *AMF3) Marshals(v ...any) []byte {
	for _, vv := range v {
		amf.Marshal(vv)
	}
	return amf.Buffer
}

func MarshalAMF3s(v ...any) []byte {
	var amf AMF3
	amf.ocEnc = make(map[uintptr]int)
	amf.scEnc = make(map[string]int)
	return amf.Marshals(v...)
}

func (amf *AMF3) Marshal(v any) []byte {
	if v == nil {
		amf.WriteByte(AMF3_NULL)
		return amf.Buffer
	}
	switch vv := v.(type) {
	case string:
		amf.WriteByte(AMF3_STRING)
		amf.writeString(vv)
	case bool:
		if vv {
			amf.WriteByte(AMF3_TRUE)
		} else {
			amf.WriteByte(AMF3_FALSE)
		}
	case int, int8, int16, int32, int64:
		var value int64
		reflect.ValueOf(&value).Elem().Set(reflect.ValueOf(vv).Convert(reflect.TypeOf(value)))
		if value < -0xfffffff {
			if value > -0x7fffffff {
				return amf.Marshal(float64(value))
			}
			return amf.Marshal(strconv.FormatInt(value, 10))
		}
		amf.WriteByte(AMF3_INTEGER)
		amf.writeU29(uint32(value))
	case uint, uint8, uint16, uint32, uint64:
		var value uint64
		reflect.ValueOf(&value).Elem().Set(reflect.ValueOf(vv).Convert(reflect.TypeOf(value)))
		if value >= 0x20000000 {
			if value <= 0xffffffff {
				return amf.Marshal(float64(value))
			}
			return amf.Marshal(strconv.FormatUint(value, 10))
		}
		amf.WriteByte(AMF3_INTEGER)
		amf.writeU29(uint32(value))
	case float32:
		amf.Marshal(float64(vv))
	case float64:
		amf.WriteByte(AMF3_DOUBLE)
		amf.WriteFloat64(vv)
	case map[string]any:
		amf.WriteByte(AMF3_OBJECT)
		index, ok := amf.ocEnc[reflect.ValueOf(vv).Pointer()]
		if ok {
			index <<= 1
			amf.writeU29(uint32(index << 1))
			return nil
		}
		amf.WriteByte(0x0b)
		err := amf.writeString("")
		if err != nil {
			return nil
		}
		for k, v := range vv {
			err = amf.writeString(k)
			if err != nil {
				return nil
			}
			amf.Marshal(v)
		}
		amf.writeString("")

	default:
		v := reflect.ValueOf(vv)
		if !v.IsValid() {
			amf.WriteByte(AMF3_NULL)
			return amf.Buffer
		}
		switch v.Kind() {
		case reflect.Ptr:
			if v.IsNil() {
				amf.WriteByte(AMF3_NULL)
				return amf.Buffer
			}
			vv := reflect.Indirect(v)
			if vv.Kind() == reflect.Struct {
				amf.WriteByte(AMF3_OBJECT)
				index, ok := amf.ocEnc[v.Pointer()]
				if ok {
					index <<= 1
					amf.writeU29(uint32(index << 1))
					return nil
				}
				amf.WriteByte(0x0b)
				err := amf.writeString("")
				if err != nil {
					return nil
				}
				t := vv.Type()
				for i := 0; i < t.NumField(); i++ {
					f := t.Field(i)
					key := amf.getFieldName(f)
					if key == "" {
						continue
					}

					err = amf.writeString(key)
					if err != nil {
						return nil
					}

					fv := v.FieldByName(f.Name)
					if fv.Kind() == reflect.Struct {
						fv = fv.Addr()
					}
					amf.Marshal(fv.Interface())
				}
				amf.writeString("")
			}
		}
	}
	return amf.Buffer
}

func (amf *AMF3) getFieldName(f reflect.StructField) string {
	chars := []rune(f.Name)
	if unicode.IsLower(chars[0]) {
		return ""
	}

	name := f.Tag.Get("amf.name")
	if name != "" {
		return name
	}

	if !amf.reservStruct {
		chars[0] = unicode.ToLower(chars[0])
		return string(chars)
	}

	return f.Name
}
