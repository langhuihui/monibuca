package pkg

type CodecCtx struct {
}
type IRaw interface {
}
type IVideoData interface {
	ToRaw(*CodecCtx) IRaw
	FromRaw(*CodecCtx, IRaw)
}

type IAudioData interface {
	ToRaw(*CodecCtx) IRaw
	FromRaw(*CodecCtx, IRaw)
}

type IData interface {
}

type H264Nalu struct {
}

func (nalu *H264Nalu) ToRaw(ctx *CodecCtx) IRaw {
	return nalu
}

func (nalu *H264Nalu) FromRaw(ctx *CodecCtx, raw IRaw) {
	*nalu = *raw.(*H264Nalu)
}

type H265Nalu struct {
}
