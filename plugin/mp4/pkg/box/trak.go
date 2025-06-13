package box

import (
	"bytes"
	"io"
)

type TrakBox struct {
	BaseBox
	MDIA *MdiaBox
	EDTS *EdtsBox
	TKHD *TrackHeaderBox
}

func CreateTrakBox(mdia *MdiaBox, edts *EdtsBox, tkhd *TrackHeaderBox) *TrakBox {
	size := uint32(BasicBoxLen)
	if mdia != nil {
		size += mdia.size
	}
	if edts != nil {
		size += edts.size
	}
	if tkhd != nil {
		size += tkhd.size
	}
	return &TrakBox{
		BaseBox: BaseBox{
			typ:  TypeTRAK,
			size: size,
		},
		MDIA: mdia,
		EDTS: edts,
		TKHD: tkhd,
	}
}

func (t *TrakBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, t.MDIA, t.EDTS, t.TKHD)
}

func (t *TrakBox) Unmarshal(buf []byte) (b IBox, err error) {
	r := bytes.NewReader(buf)
	for err == nil {
		b, err = ReadFrom(r)
		switch box := b.(type) {
		case *MdiaBox:
			t.MDIA = box
		case *EdtsBox:
			t.EDTS = box
		case *TrackHeaderBox:
			t.TKHD = box
		}
	}
	return t, err
}

// SampleCallback 定义样本处理回调函数类型
type SampleCallback func(sample *Sample, sampleIndex int) error

// ParseSamples parses the sample table and builds the sample list
func (t *TrakBox) ParseSamples() (samplelist []Sample) {
	return t.ParseSamplesWithCallback(nil)
}

// ParseSamplesWithCallback parses the sample table and builds the sample list with optional callback
func (t *TrakBox) ParseSamplesWithCallback(callback SampleCallback) (samplelist []Sample) {
	stbl := t.MDIA.MINF.STBL
	var chunkOffsets []uint64
	if stbl.STCO != nil {
		chunkOffsets = stbl.STCO.Entries
	}

	var sampleToChunks []STSCEntry
	if stbl.STSC != nil {
		sampleToChunks = stbl.STSC.Entries
	}

	var sampleCount uint32
	if stbl.STSZ != nil {
		sampleCount = stbl.STSZ.SampleCount
	}

	samplelist = make([]Sample, sampleCount)
	iterator := 0

	for i, chunk := range sampleToChunks {
		var samplesInChunk uint32
		if i < len(sampleToChunks)-1 {
			samplesInChunk = sampleToChunks[i+1].FirstChunk - chunk.FirstChunk
		} else {
			samplesInChunk = uint32(len(chunkOffsets)) - chunk.FirstChunk + 1
		}

		for j := uint32(0); j < samplesInChunk; j++ {
			chunkIndex := chunk.FirstChunk - 1 + j
			if chunkIndex >= uint32(len(chunkOffsets)) {
				break
			}

			for k := uint32(0); k < chunk.SamplesPerChunk; k++ {
				if iterator >= len(samplelist) {
					break
				}

				sample := &samplelist[iterator]
				if stbl.STSZ != nil {
					if stbl.STSZ.SampleSize != 0 {
						sample.Size = int(stbl.STSZ.SampleSize)
					} else {
						sample.Size = int(stbl.STSZ.EntrySizelist[iterator])
					}
				}

				sample.Offset = int64(chunkOffsets[chunkIndex])
				if k > 0 {
					sample.Offset = samplelist[iterator-1].Offset + int64(samplelist[iterator-1].Size)
				}

				iterator++
			}
		}
	}

	// Process STTS entries for timestamps
	if stbl.STTS != nil {
		sampleIndex := 0
		timestamp := uint32(0)

		for _, entry := range stbl.STTS.Entries {
			for i := uint32(0); i < entry.SampleCount; i++ {
				if sampleIndex >= len(samplelist) {
					break
				}
				samplelist[sampleIndex].Timestamp = timestamp
				timestamp += entry.SampleDelta
				sampleIndex++
			}
		}
	}

	// Process CTTS entries for presentation timestamps
	if stbl.CTTS != nil {
		sampleIndex := 0
		for _, entry := range stbl.CTTS.Entries {
			for i := uint32(0); i < entry.SampleCount; i++ {
				if sampleIndex >= len(samplelist) {
					break
				}
				samplelist[sampleIndex].CTS = entry.SampleOffset
				sampleIndex++
			}
		}
	}

	if stbl.STSS != nil {
		for _, keyIndex := range stbl.STSS.Entries {
			samplelist[keyIndex-1].KeyFrame = true
		}
	}

	// 调用回调函数处理每个样本
	if callback != nil {
		for i := range samplelist {
			if err := callback(&samplelist[i], i); err != nil {
				// 如果回调返回错误，可以选择记录或处理，但不中断解析
				// 这里为了保持向后兼容性，我们继续处理
				continue
			}
		}
	}

	return samplelist
}

func init() {
	RegisterBox[*TrakBox](TypeTRAK)
}
