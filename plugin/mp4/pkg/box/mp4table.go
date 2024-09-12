package box

//based on ffmpeg

type STTSEntry struct {
	SampleCount uint32
	SampleDelta uint32
}

type SubSampleEntry struct {
	BytesOfClearData     uint16
	BytesOfProtectedData uint32
}

type SencEntry struct {
	IV         []byte
	SubSamples []SubSampleEntry
}

type CTTSEntry struct {
	SampleCount  uint32
	SampleOffset uint32
}

type STSCEntry struct {
	FirstChunk             uint32
	SamplesPerChunk        uint32
	SampleDescriptionIndex uint32
}

type ELSTEntry struct {
	SegmentDuration   uint64
	MediaTime         int64
	MediaRateInteger  int16
	MediaRateFraction int16
}

type TrunEntry struct {
	SampleDuration              uint32
	SampleSize                  uint32
	SampleFlags                 uint32
	SampleCompositionTimeOffset uint32
}

type SENC struct {
	Entrys []SencEntry
}

type FragEntry struct {
	Time       uint64
	MoofOffset uint64
}
