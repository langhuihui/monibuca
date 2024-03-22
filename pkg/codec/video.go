package codec

type SPSInfo struct {
	ProfileIdc uint
	LevelIdc   uint

	MbWidth  uint
	MbHeight uint

	CropLeft   uint
	CropRight  uint
	CropTop    uint
	CropBottom uint

	Width  uint
	Height uint
}



type AudioCodecID byte
type VideoCodecID byte

const (
	ADTS_HEADER_SIZE              = 7
	CodecID_AAC      AudioCodecID = 0xA
	CodecID_PCMA     AudioCodecID = 7
	CodecID_PCMU     AudioCodecID = 8
	CodecID_OPUS     AudioCodecID = 0xC
	CodecID_H264     VideoCodecID = 7
	CodecID_H265     VideoCodecID = 0xC
	CodecID_AV1      VideoCodecID = 0xD
)

func (codecId AudioCodecID) String() string {
	switch codecId {
	case CodecID_AAC:
		return "aac"
	case CodecID_PCMA:
		return "pcma"
	case CodecID_PCMU:
		return "pcmu"
	case CodecID_OPUS:
		return "opus"
	}
	return "unknow"
}

func (codecId VideoCodecID) String() string {
	switch codecId {
	case CodecID_H264:
		return "h264"
	case CodecID_H265:
		return "h265"
	case CodecID_AV1:
		return "av1"
	}
	return "unknow"
}
