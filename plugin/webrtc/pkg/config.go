package webrtc

import (
	. "github.com/pion/webrtc/v4"
)

var videoRTCPFeedback = []RTCPFeedback{{"goog-remb", ""}, {"ccm", "fir"}, {"nack", ""}, {"nack", "pli"}, {"transport-cc", ""}}

type CodecWithType struct {
	Codec RTPCodecParameters
	Type  RTPCodecType
}

func GetDefaultCodecs() ([]CodecWithType, error) {
	var codecs []CodecWithType

	// 音频编解码器
	for _, codec := range []RTPCodecParameters{
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypePCMU, 8000, 0, "", nil},
			PayloadType:        0,
		},
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypePCMA, 8000, 0, "", nil},
			PayloadType:        8,
		},
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
			PayloadType:        111,
		},
	} {
		codecs = append(codecs, CodecWithType{Codec: codec, Type: RTPCodecTypeAudio})
	}

	// 视频编解码器
	for _, codec := range []RTPCodecParameters{
		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=96", nil},
		// 	PayloadType:        97,
		// },

		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=98", nil},
		// 	PayloadType:        99,
		// },

		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=100", nil},
		// 	PayloadType:        101,
		// },
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f", videoRTCPFeedback},
			PayloadType:        102,
		},
		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=102", nil},
		// 	PayloadType:        121,
		// },
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=64001f", videoRTCPFeedback},
			PayloadType:        112,
		},
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f", videoRTCPFeedback},
			PayloadType:        127,
		},
		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=127", nil},
		// 	PayloadType:        120,
		// },

		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f", videoRTCPFeedback},
			PayloadType:        125,
		},
		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=125", nil},
		// 	PayloadType:        107,
		// },

		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42e01f", videoRTCPFeedback},
			PayloadType:        108,
		},
		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=108", nil},
		// 	PayloadType:        109,
		// },

		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f", videoRTCPFeedback},
			PayloadType:        127,
		},
		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=127", nil},
		// 	PayloadType:        120,
		// },

		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH264, 90000, 0, "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032", videoRTCPFeedback},
			PayloadType:        123,
		},
		// {
		// 	RTPCodecCapability: RTPCodecCapability{"video/rtx", 90000, 0, "apt=123", nil},
		// 	PayloadType:        118,
		// },
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH265, 90000, 0, "level-id=180;profile-id=1;tier-flag=0;tx-mode=SRST", videoRTCPFeedback},
			PayloadType:        49,
		},
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH265, 90000, 0, "level-id=186;profile-id=1;tier-flag=0;tx-mode=SRST", videoRTCPFeedback},
			PayloadType:        50,
		},
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH265, 90000, 0, "level-id=180;profile-id=2;tier-flag=0;tx-mode=SRST", videoRTCPFeedback},
			PayloadType:        51,
		},
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeH265, 90000, 0, "level-id=186;profile-id=2;tier-flag=0;tx-mode=SRST", videoRTCPFeedback},
			PayloadType:        52,
		},
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeVP9, 90000, 0, "", nil},
			PayloadType:        100,
		},
		{
			RTPCodecCapability: RTPCodecCapability{MimeTypeAV1, 90000, 0, "profile=2;level-idx=8;tier=1", nil},
			PayloadType:        45,
		},
	} {
		codecs = append(codecs, CodecWithType{Codec: codec, Type: RTPCodecTypeVideo})
	}
	return codecs, nil
}
