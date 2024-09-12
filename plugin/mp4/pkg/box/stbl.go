package box

type SampleTable struct {
	STTS *TimeToSampleBox
	CTTS *CompositionOffsetBox
	STSC *SampleToChunkBox
	STSZ *SampleSizeBox
	STCO *ChunkOffsetBox
	STSS *SyncSampleBox
}
