package codec

type H26XCtx struct {
	VPS, SPS, PPS []byte
}

func (ctx *H26XCtx) FourCC() (f FourCC) {
	return
}

func (ctx *H26XCtx) GetInfo() string {
	return ""
}

func (ctx *H26XCtx) GetBase() ICodecCtx {
	return ctx
}

func (ctx *H26XCtx) GetRecord() []byte {
	return nil
}

func (ctx *H26XCtx) String() string {
	return ""
}
