package flv

const (
	// FLV Tag Type
	FLV_TAG_TYPE_AUDIO  = 0x08
	FLV_TAG_TYPE_VIDEO  = 0x09
	FLV_TAG_TYPE_SCRIPT = 0x12
)

func WriteFLVTag(t uint8, ts, dataSize uint32, b []byte) {
	b[0] = t
	b[1], b[2], b[3] = byte(dataSize>>16), byte(dataSize>>8), byte(dataSize)
	b[4], b[5], b[6], b[7] = byte(ts>>16), byte(ts>>8), byte(ts), byte(ts>>24)
}
