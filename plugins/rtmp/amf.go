package rtmp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/langhuihui/monibuca/monica/pool"
	"github.com/langhuihui/monibuca/monica/util"
	"log"
	"reflect"
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
	AMF0_NUMBER         = 0x00 // 浮点数
	AMF0_BOOLEAN        = 0x01 // 布尔型
	AMF0_STRING         = 0x02 // 字符串
	AMF0_OBJECT         = 0x03 // 对象,开始
	AMF0_MOVIECLIP      = 0x04
	AMF0_NULL           = 0x05 // null
	AMF0_UNDEFINED      = 0x06
	AMF0_REFERENCE      = 0x07
	AMF0_ECMA_ARRAY     = 0x08
	AMF0_END_OBJECT     = 0x09 // 对象,结束
	AMF0_STRICT_ARRAY   = 0x0A
	AMF0_DATE           = 0x0B // 日期
	AMF0_LONG_STRING    = 0x0C // 字符串
	AMF0_UNSUPPORTED    = 0x0D
	AMF0_RECORDSET      = 0x0E
	AMF0_XML_DOCUMENT   = 0x0F
	AMF0_TYPED_OBJECT   = 0x10
	AMF0_AVMPLUS_OBJECT = 0x11

	AMF3_UNDEFINED     = 0x00
	AMF3_NULL          = 0x01
	AMF3_FALSE         = 0x02
	AMF3_TRUE          = 0x03
	AMF3_INTEGER       = 0x04
	AMF3_DOUBLE        = 0x05
	AMF3_STRING        = 0x06
	AMF3_XML_DOC       = 0x07
	AMF3_DATE          = 0x08
	AMF3_ARRAY         = 0x09
	AMF3_OBJECT        = 0x0A
	AMF3_XML           = 0x0B
	AMF3_BYTE_ARRAY    = 0x0C
	AMF3_VECTOR_INT    = 0x0D
	AMF3_VECTOR_UINT   = 0x0E
	AMF3_VECTOR_DOUBLE = 0x0F
	AMF3_VECTOR_OBJECT = 0x10
	AMF3_DICTIONARY    = 0x11
)

var END_OBJ = []byte{0, 0, AMF0_END_OBJECT}

type AMFObject interface{}

type AMFObjects map[string]AMFObject

func newAMFObjects() AMFObjects {
	return make(AMFObjects, 0)
}

func DecodeAMFObject(obj interface{}, key string) interface{} {
	if v, ok := obj.(AMFObjects)[key]; ok {
		return v
	}
	return nil
}

type AMF struct {
	*bytes.Buffer
}

func newAMFEncoder() *AMF {
	return &AMF{
		new(bytes.Buffer),
	}
}

func newAMFDecoder(b []byte) *AMF {
	return &AMF{
		bytes.NewBuffer(b),
	}
}
func (amf *AMF) readSize() (int, error) {
	b, err := readBytes(amf.Buffer, 4)
	size := int(util.BigEndian.Uint32(b))
	pool.RecycleSlice(b)
	return size, err
}
func (amf *AMF) readSize16() (int, error) {
	b, err := readBytes(amf.Buffer, 2)
	size := int(util.BigEndian.Uint16(b))
	pool.RecycleSlice(b)
	return size, err
}
func (amf *AMF) readObjects() (obj []AMFObject, err error) {
	obj = make([]AMFObject, 0)

	for amf.Len() > 0 {
		if v, err := amf.decodeObject(); err == nil {
			obj = append(obj, v)
		} else {
			return obj, err
		}
	}
	return
}

func (amf *AMF) writeObjects(obj []AMFObject) (err error) {
	for _, v := range obj {
		switch data := v.(type) {
		case string:
			err = amf.writeString(data)
		case float64:
			err = amf.writeNumber(data)
		case bool:
			err = amf.writeBool(data)
		case AMFObjects:
			err = amf.encodeObject(data)
		case nil:
			err = amf.writeNull()
		default:
			log.Printf("amf encode unknown type:%v", reflect.TypeOf(data).Name())
		}
	}

	return
}

func (amf *AMF) decodeObject() (obj AMFObject, err error) {
	if amf.Len() == 0 {
		return nil, errors.New(fmt.Sprintf("no enough bytes, %v/%v", amf.Len(), 1))
	}
	var t byte
	if t, err = amf.ReadByte(); err != nil {
		return
	}
	if err = amf.UnreadByte(); err != nil {
		return
	}
	switch t {
	case AMF0_NUMBER:
		return amf.readNumber()
	case AMF0_BOOLEAN:
		return amf.readBool()
	case AMF0_STRING:
		return amf.readString()
	case AMF0_OBJECT:
		return amf.readObject()
	case AMF0_MOVIECLIP:
		log.Println("This type is not supported and is reserved for future use.(AMF0_MOVIECLIP)")
	case AMF0_NULL:
		return amf.readNull()
	case AMF0_UNDEFINED:
		_, err = amf.ReadByte()
		return "Undefined", err
	case AMF0_REFERENCE:
		log.Println("reference-type.(AMF0_REFERENCE)")
	case AMF0_ECMA_ARRAY:
		return amf.readECMAArray()
	case AMF0_END_OBJECT:
		_, err = amf.ReadByte()
		return "ObjectEnd", err
	case AMF0_STRICT_ARRAY:
		return amf.readStrictArray()
	case AMF0_DATE:
		return amf.readDate()
	case AMF0_LONG_STRING:
		return amf.readLongString()
	case AMF0_UNSUPPORTED:
		log.Println("If a type cannot be serialized a special unsupported marker can be used in place of the type.(AMF0_UNSUPPORTED)")
	case AMF0_RECORDSET:
		log.Println("This type is not supported and is reserved for future use.(AMF0_RECORDSET)")
	case AMF0_XML_DOCUMENT:
		return amf.readLongString()
	case AMF0_TYPED_OBJECT:
		log.Println("If a strongly typed object has an alias registered for its class then the type name will also be serialized. Typed objects are considered complex types and reoccurring instances can be sent by reference.(AMF0_TYPED_OBJECT)")
	case AMF0_AVMPLUS_OBJECT:
		log.Println("AMF0_AVMPLUS_OBJECT")
	default:
		log.Println("Unkonw type.")
	}
	return nil, errors.New(fmt.Sprintf("Unsupported type %v", t))
}

func (amf *AMF) encodeObject(t AMFObjects) (err error) {
	err = amf.writeObject()
	defer amf.writeObjectEnd()
	for k, vv := range t {
		switch vvv := vv.(type) {
		case string:
			if err = amf.writeObjectString(k, vvv); err != nil {
				return
			}
		case float64, uint, float32, int, int16, int32, int64, uint16, uint32, uint64, uint8, int8:
			if err = amf.writeObjectNumber(k, util.ToFloat64(vvv)); err != nil {
				return
			}
		case bool:
			if err = amf.writeObjectBool(k, vvv); err != nil {
				return
			}
		}
	}
	return
}

func (amf *AMF) readDate() (t uint64, err error) {
	_, err = amf.ReadByte() // 取出第一个字节 8 Bit == 1 Byte. buf - 1.
	var b []byte
	b, err = readBytes(amf.Buffer, 8) // 在取出8个字节,并且读到b中. buf - 8
	t = util.BigEndian.Uint64(b)
	pool.RecycleSlice(b)
	b, err = readBytes(amf.Buffer, 2)
	pool.RecycleSlice(b)
	return t, err
}

func (amf *AMF) readStrictArray() (list []AMFObject, err error) {
	list = make([]AMFObject, 0)
	_, err = amf.ReadByte()
	var size int
	size, err = amf.readSize()
	for i := 0; i < size; i++ {
		if obj, err := amf.decodeObject(); err != nil {
			return list, err
		} else {
			list = append(list, obj)
		}
	}
	return
}

func (amf *AMF) readECMAArray() (m AMFObjects, err error) {
	m = make(AMFObjects, 0)
	_, err = amf.ReadByte()
	var size int
	size, err = amf.readSize()
	for i := 0; i < size; i++ {
		var k string
		var v AMFObject
		if k, err = amf.readString1(); err == nil {
			if v, err = amf.decodeObject(); err == nil {
				if k != "" || "ObjectEnd" != v {
					m[k] = v
					continue
				}
			}
		}
		return
	}
	return
}

func (amf *AMF) readString() (str string, err error) {
	_, err = amf.ReadByte() // 取出第一个字节 8 Bit == 1 Byte. buf - 1.
	return amf.readString1()
}

func (amf *AMF) readString1() (str string, err error) {
	var size int
	size, err = amf.readSize16()
	var b []byte
	b, err = readBytes(amf.Buffer, size) // 读取全部数据,读取长度为l,因为这两个字节(l变量)保存的是数据长度
	str = string(b)
	pool.RecycleSlice(b)
	return
}

func (amf *AMF) readLongString() (str string, err error) {
	_, err = amf.ReadByte()
	var size int
	size, err = amf.readSize()
	var b []byte
	b, err = readBytes(amf.Buffer, size) // 读取全部数据,读取长度为l,因为这两个字节(l变量)保存的是数据长度
	str = string(b)
	pool.RecycleSlice(b)
	return
}

func (amf *AMF) readNull() (AMFObject, error) {
	_, err := amf.ReadByte()
	return nil, err
}

func (amf *AMF) readNumber() (num float64, err error) {
	// binary.read 会读取8个字节(float64),如果小于8个字节返回一个`io.ErrUnexpectedEOF`,如果大于就会返回`io.ErrShortBuffer`,读取完毕会有`io.EOF`
	_, err = amf.ReadByte()
	err = binary.Read(amf, binary.BigEndian, &num)
	return num, err
}

func (amf *AMF) readBool() (f bool, err error) {
	_, err = amf.ReadByte()
	if b, err := amf.ReadByte(); err == nil {
		return b == 1, nil
	}
	return
}

func (amf *AMF) readObject() (m AMFObjects, err error) {
	_, err = amf.ReadByte()
	m = make(AMFObjects, 0)
	var k string
	var v AMFObject
	for {
		if k, err = amf.readString1(); err != nil {
			break
		}
		if v, err = amf.decodeObject(); err != nil {
			break
		}
		if k == "" && "ObjectEnd" == v {
			break
		}
		m[k] = v
	}
	return m, err
}

func readBytes(buf *bytes.Buffer, length int) (b []byte, err error) {
	b = pool.GetSlice(length)
	if i, _ := buf.Read(b); length != i {
		err = errors.New(fmt.Sprintf("not enough bytes,%v/%v", buf.Len(), length))
	}
	return
}
func (amf *AMF) writeSize16(l int) (err error) {
	b := pool.GetSlice(2)
	defer pool.RecycleSlice(b)
	util.BigEndian.PutUint16(b, uint16(l))
	_, err = amf.Write(b)
	return
}
func (amf *AMF) writeString(value string) error {
	v := []byte(value)
	err := amf.WriteByte(byte(AMF0_STRING))
	if err != nil {
		return err
	}
	if err = amf.writeSize16(len(v)); err != nil {
		return err
	}
	_, err = amf.Write(v)
	return err
}

func (amf *AMF) writeNull() error {
	return amf.WriteByte(byte(AMF0_NULL))
}

func (amf *AMF) writeBool(b bool) error {
	if err := amf.WriteByte(byte(AMF0_BOOLEAN)); err != nil {
		return err
	}
	if b {
		return amf.WriteByte(byte(1))
	}
	return amf.WriteByte(byte(0))
}

func (amf *AMF) writeNumber(b float64) error {
	if err := amf.WriteByte(byte(AMF0_NUMBER)); err != nil {
		return err
	}
	return binary.Write(amf, binary.BigEndian, b)
}

func (amf *AMF) writeObject() error {
	return amf.WriteByte(byte(AMF0_OBJECT))
}
func (amf *AMF) writeKey(key string) (err error) {
	keyByte := []byte(key)
	if err = amf.writeSize16(len(keyByte)); err != nil {
		return
	}
	if _, err = amf.Write(keyByte); err != nil {
		return
	}
	return
}
func (amf *AMF) writeObjectString(key, value string) error {
	if err := amf.writeKey(key); err != nil {
		return err
	}
	return amf.writeString(value)
}

func (amf *AMF) writeObjectBool(key string, f bool) error {
	if err := amf.writeKey(key); err != nil {
		return err
	}
	return amf.writeBool(f)
}

func (amf *AMF) writeObjectNumber(key string, value float64) error {
	if err := amf.writeKey(key); err != nil {
		return err
	}
	return amf.writeNumber(value)
}

func (amf *AMF) writeObjectEnd() error {
	_, err := amf.Write(END_OBJ)
	return err
}
