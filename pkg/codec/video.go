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
