package codec

const (
	AV1_OBU_SEQUENCE_HEADER        = 1
	AV1_OBU_TEMPORAL_DELIMITER     = 2
	AV1_OBU_FRAME_HEADER           = 3
	AV1_OBU_TILE_GROUP             = 4
	AV1_OBU_METADATA               = 5
	AV1_OBU_FRAME                  = 6
	AV1_OBU_REDUNDANT_FRAME_HEADER = 7
	AV1_OBU_TILE_LIST              = 8
	AV1_OBU_PADDING                = 15
)

type (
	IAV1Ctx interface {
		GetAV1Ctx() *AV1Ctx
	}
	AV1Ctx struct {
		ConfigOBUs []byte
	}
)

func (ctx *AV1Ctx) GetAV1Ctx() *AV1Ctx {
	return ctx
}

func (*AV1Ctx) FourCC() FourCC {
	return FourCC_AV1
}
