package codec

type ICodecCtx interface {
	FourCC() FourCC
	GetInfo() string
	GetBase() ICodecCtx
}
