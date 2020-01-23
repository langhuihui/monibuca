package avformat

import (
	"github.com/langhuihui/monibuca/monica/util"
)

type MP4 interface {
}

type MP4Box interface {
	Header() *MP4Header
	Body() *MP4Body
}

//
// ISO_IEC_14496-12_2012.pdf Page/17
//
// The standard boxes all use compact types (32-bit) and most boxes will use the compact (32-bit) size
// standard header
type MP4BoxHeader struct {
	BoxSize uint32 // 32 bits, is an integer that specifies the number of bytes in this box, including all its fields and contained boxes; if size is 1 then the actual size is in the field largesize; if size is 0, then this box is the last one in the file, and its contents extend to the end of the file (normally only used for a Media Data Box)
	BoxType uint32 // 32 bits, identifies the box type; standard boxes use a compact type, which is normally four printable characters, to permit ease of identification, and is shown so in the boxes below. User extensions use an extended type; in this case, the type field is set to ‘uuid’.
}

//
// ISO_IEC_14496-12_2012.pdf Page/17
//
// Many objects also contain a version number and flags field
// full box header
type MP4FullBoxHeader struct {
	Version uint8   // 8 bits, is an integer that specifies the version of this format of the box.
	Flags   [3]byte // 24 bits, is a map of flags
}

//
// ISO_IEC_14496-12_2012.pdf Page/17
//
// Typically only the Media Data Box(es) need the 64-bit size.
// lagesize box header
type MP4BoxLargeHeader struct {
	LargeSize uint64    // 64 bits
	UUIDs     [16]uint8 // 128 bits
}

// if(size == 1)
// {
// 	unsigned int(64) largesize;
// }
// else if(size == 0)
// {
// 	// box extends to end of file
// }
// if(boxtype == ‘uuid’)
// {
// 	unsigned int(8)[16] usertype = extended_type;
// }

type MP4Header struct {
	MP4BoxHeader
}

type MP4Body struct{}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/18
//
// Box Type: ftyp
// Container: File
// Mandatory: Yes
// Quantity: Exactly one (but see below)
//
// Each brand is a printable four-character code, registered with ISO, that identifies a precise specification
type FileTypeBox struct {
	MP4BoxHeader // standard header

	MajorBrand       uint32   // 32 bits, is a brand identifier
	MinorVersion     uint32   // 32 bits, is an informative integer for the minor version of the major brand
	CompatibleBrands []uint32 // 32 bits array, is a list, to the end of the box, of brands
}

func NewFileTypeBox() (box *FileTypeBox) {
	box = new(FileTypeBox)
	box.MP4BoxHeader.BoxType, _ = util.ByteToUint32([]byte("ftyp"), true)

	return
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/30
//
// Box Types: pdin
// Container: File
// Mandatory: No
// Quantity: Zero or One
type ProgressiveDownloadInformationBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Rate         uint32 // 32 bits, is a download rate expressed in bytes/second
	InitialDelay uint32 // 32 bits, is the suggested delay to use when playing the file, such that if download continues at the given rate, all data within the file will arrive in time for its use and playback should not need to stall.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/30
//
// Box Type: moov
// Container: File
// Mandatory: Yes
// Quantity: Exactly one
//
// The metadata for a presentation is stored in the single Movie Box which occurs at the top-level of a file.
// Normally this box is close to the beginning or end of the file, though this is not required
type MovieBox struct {
	MP4BoxHeader // standard header

	//Mhb MovieHeaderBox // the first child box(header box)
}

func NewMovieBox() (box *MovieBox) {
	box = new(MovieBox)
	box.MP4BoxHeader.BoxType, _ = util.ByteToUint32([]byte("moov"), true)

	return
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/31
//
// Box Type: mvhd
// Container: Movie Box ('moov')
// Mandatory: Yes
// Quantity: Exactly one
//
// This box defines overall information which is media-independent, and relevant to the entire presentation
// considered as a whole
type MovieHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	CreationTime     interface{} // uint64 or uint32, is an integer that declares the creation time of the presentation (in seconds since midnight, Jan. 1, 1904, in UTC time)
	ModificationTime interface{} // uint64 or uint32, is an integer that declares the most recent time the presentation was modified (in seconds since midnight, Jan. 1, 1904, in UTC time)
	TimeScale        uint32      // 32 bits, is an integer that specifies the time-scale for the entire presentation; this is the number of time units that pass in one second. For example, a time coordinate system that measures time in sixtieths of a second has a time scale of 60.
	Duration         interface{} // uint64 or uint32, is an integer that declares length of the presentation (in the indicated timescale). This property is derived from the presentation's tracks: the value of this field corresponds to the duration of the longest track in the presentation. If the duration cannot be determined then duration is set to all 1s.
	Rate             int32       // 32 bits, is a fixed point 16.16 number that indicates the preferred rate to play the presentation; 1.0 (0x00010000) is normal forward playback
	Volume           int16       // 16 bits, is a fixed point 8.8 number that indicates the preferred playback volume. 1.0 (0x0100) is full volume.
	Reserved1        int16       // 16 bits, bit[16]
	Reserved2        [2]uint32   // 32 bits array, const unsigned int(32)[2]
	Matrix           [9]int32    // 32 bits array, provides a transformation matrix for the video; (u,v,w) are restricted here to (0,0,1), hex values(0,0,0x40000000).
	PreDefined       [6]int32    // 32 bits array, bit(32)[6]
	NextTrackID      uint32      // 32 bits, is a non-zero integer that indicates a value to use for the track ID of the next track to be added to this presentation. Zero is not a valid track ID value. The value of next_track_ID shall be larger than the largest track-ID in use. If this value is equal to all 1s (32-bit maxint), and a new media track is to be added, then a search must be made in the file for an unused track identifier.
}

// CreationTime 	: 创建时间(相对于UTC时间1904-01-01零点的秒数)
// ModificationTime : 修改时间
// TimeScale 		: 文件媒体在1秒时间内的刻度值，可以理解为1秒长度的时间单元数
// Duration 		: 该track的时间长度，用duration和time scale值可以计算track时长，比如audio track的time scale = 8000, duration = 560128，时长为70.016，video track的time scale = 600, duration = 42000，时长为70
// Rate 			: 推荐播放速率，高16位和低16位分别为小数点整数部分和小数部分，即[16.16] 格式，该值为1.0（0x00010000）表示正常前向播放
// Volume 			: 与rate类似，[8.8] 格式，1.0（0x0100）表示最大音量
// Matrix 			: 视频变换矩阵 { 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 }
// NextTrackID 		: 下一个track使用的id号

// PreDefined:
// Preview Time 		: 开始预览此movie的时间
// Preview Duration 	: 以movie的time scale为单位，预览的duration
// Poster Time 			: The time value of the time of the movie poster.
// Selection Time 		: The time value for the start time of the current selection.
// Selection Duration 	: The duration of the current selection in movie time scale units.
// Current Time 		: 当前时间

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/32
//
// Box Type: trak
// Container: Movie Box ('moov')
// Mandatory: Yes
// Quantity: One or more
type TrackBox struct {
	MP4BoxHeader // standard header

	Thb TrackHeaderBox // the first child box(header box)
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/32
//
// Box Type: tkhd
// Container: Track Box ('trak')
// Mandatory: Yes
// Quantity: Exactly one
type TrackHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	CreationTime     interface{} // uint64 or uint32,
	ModificationTime interface{} // uint64 or uint32,
	TrackID          uint32      // 32 bits, is an integer that uniquely identifies this track over the entire life-time of this presentation. Track IDs are never re-used and cannot be zero
	Reserved1        uint32      // 32 bits,
	Duration         interface{} // uint64 or uint32,
	Reserved2        [2]uint32   // 32 bits array,
	Layer            int16       // 16 bits, specifies the front-to-back ordering of video tracks; tracks with lower numbers are closer to the viewer. 0 is the normal value, and -1 would be in front of track 0, and so on
	AlternateGroup   int16       // 16 bits,
	Volume           int16       // 16 bits, if track_is_audio 0x0100 else 0
	Reserved3        uint16      // 16 bits,
	Matrix           [9]int32    // 32 bits array, provides a transformation matrix for the video; (u,v,w) are restricted here to (0,0,1), hex (0,0,0x40000000). { 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 }
	Width            uint32      // 32 bits,
	Height           uint32      // 32 bits,
}

// CreationTime     : 创建时间
// ModificationTime : 修改时间
// TrackID          : id号，不能重复且不能为0
// Reserved1        : 保留位
// Duration         : track的时间长度
// Reserved2        : 保留位
// Layer            : 视频层，默认为0，值小的在上层
// AlternateGroup   : track分组信息，默认为0表示该track未与其他track有群组关系
// Volume           : [8.8] 格式，如果为音频track，1.0（0x0100）表示最大音量；否则为0
// Reserved3        : 保留位
// Matrix           : 视频变换矩阵 { 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 }
// Width            : 宽
// Height           : 高，均为 [16.16] 格式值，与sample描述中的实际画面大小比值，用于播放时的展示宽高

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/34
//
// Box Type: tref
// Container: Track Box (‘trak’)
// Mandatory: No
// Quantity: Zero or one
type TrackReferenceBox struct {
	MP4BoxHeader // standard header
}

type TrackReferenceTypeBox struct {
	MP4BoxHeader // standard header

	TrackIDs []uint32 // 32 bits, is an integer that provides a reference from the containing track to another track in the presentation. track_IDs are never re-used and cannot be equal to zero
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/35
//
// Box Type: trgr
// Container: Track Box (‘trak’)
// Mandatory: No
// Quantity: Zero or one
type TrackGroupBox struct {
	MP4BoxHeader // standard header
}

type TrackGroupTypeBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	TrackGroupID uint32 // 32 bits, indicates the grouping type and shall be set to one of the following values, or a value registered, or a value from a derived specification or registration
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/54
//
// Box Type: edts
// Container: Track Box (‘trak’)
// Mandatory: No
// Quantity: Zero or one
type EditBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/54
//
// Box Type: elst
// Container: Edit Box (‘edts’)
// Mandatory: No
// Quantity: Zero or one
type EditListBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint32          // 32 bits, is an integer that gives the number of entries in the following table
	Tables     []EditListTable // Edit List Table
}

type EditListTable struct {
	SegmentDuration   interface{} // uint64 or uint32, is an integer that specifies the duration of this edit segment in units of the timescale in the Movie Header Box
	MediaTime         interface{} // uint64 or uint32, is an integer containing the starting time within the media of this edit segment (in media time scale units, in composition time). If this field is set to –1, it is an empty edit. The last edit in a track shall never be an empty edit. Any difference between the duration in the Movie Header Box, and the track’s duration is expressed as an implicit empty edit at the end.
	MediaRateInteger  int16       // 16 bits,
	MediaRateFraction int16       // 16 bits,
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/36
//
// Box Type: mdia
// Container: Track Box ('trak')
// Mandatory: Yes
// Quantity: Exactly one
//
// The media declaration container contains all the objects that declare information about the media data within a track.
type MediaBox struct {
	MP4BoxHeader // standard header

	Mhb MediaHeaderBox // the first child box(header box)
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/36
//
// Box Type: mdhd
// Container: Media Box ('mdia')
// Mandatory: Yes
// Quantity: Exactly one
//
// The media header declares overall information that is media-independent, and relevant to characteristics of the media in a track.
type MediaHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	CreationTime     interface{} // int64 or int32, is an integer that declares the creation time of the presentation (in seconds since midnight, Jan. 1, 1904, in UTC time)
	ModificationTime interface{} // int64 or int32, is an integer that declares the most recent time the presentation was modified (in seconds since midnight, Jan. 1, 1904, in UTC time)
	TimeScale        uint32      // 32 bits, is an integer that specifies the time-scale for the entire presentation; this is the number of time units that pass in one second. For example, a time coordinate system that measures time in sixtieths of a second has a time scale of 60.
	Duration         interface{} // int64 or int32, is an integer that declares length of the presentation (in the indicated timescale). This property is derived from the presentation's tracks: the value of this field corresponds to the duration of the longest track in the presentation. If the duration cannot be determined then duration is set to all 1s.
	Pad              byte        // 1 bit,
	Language         [2]byte     // 15 bits, unsigned int(5)[3], declares the language code for this media. See ISO 639-2/T for the set of three charactercodes. Each character is packed as the difference between its ASCII value and 0x60. Since the code is confined to being three lower-case letters, these values are strictly positive
	PreDefined       uint16      // 16 bits,
}

// Language		: 媒体的语言码
// PreDefined	: 媒体的回放质量？？？怎样生成此质量，什么是参照点

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/37
//
// Box Type: hdlr
// Container: Media Box ('mdia') or Meta Box ('meta')
// Mandatory: Yes
// Quantity: Exactly one
type HandlerBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	PreDefined  uint32    // 32 bits,
	HandlerType uint32    // 32 bits, when present in a meta box, contains an appropriate value to indicate the format of the meta box contents. The value 'null' can be used in the primary meta box to indicate that it is merely being used to hold resources
	Reserved    [3]uint32 // 32 bits,
	Name        string    // string, is a null-terminated string in UTF-8 characters which gives a human-readable name for the track type (for debugging and inspection purposes).
}

// handler_type when present in a media box, is an integer containing one of the following values, or a value from a derived specification:
// 'vide' Video track
// 'soun' Audio track
// 'hint' Hint track
// 'meta' Timed Metadata track
// 'auxv' Auxiliary Video track

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/38
//
// Box Type: minf
// Container: Media Box ('mdia')
// Mandatory: Yes
// Quantity: Exactly one
//
// This box contains all the objects that declare characteristic information of the media in the track.
type MediaInformationBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/38
//
// Box Types: vmhd, smhd, hmhd, nmhd
// Container: Media Information Box (‘minf’)
// Mandatory: Yes
// Quantity: Exactly one specific media header shall be present
//
// There is a different media information header for each track type (corresponding to the media handler-type);
// the matching header shall be present, which may be one of those defined here, or one defined in a derived specification
type MediaInformationHeaderBoxes struct {
	// VideoMediaHeaderBox
	//
}

// Box Types: vmhd
// The video media header contains general presentation information, independent of the coding, for video media.
// Note that the flags field has the value 1.
type VideoMediaHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	GraphicsMode uint16    // 16 bits, specifies a composition mode for this video track, from the following enumerated set, which may be extended by derived specifications: copy = 0 copy over the existing image
	Opcolor      [3]uint16 // 16 bits array, is a set of 3 colour values (red, green, blue) available for use by graphics modes
}

// Box Types: smhd
// The sound media header contains general presentation information, independent of the coding, for audio media.
// This header is used for all tracks containing audio.
type SoundMediaHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Balance  int16  // 16 bits, is a fixed-point 8.8 number that places mono audio tracks in a stereo space; 0 is centre (the normal value); full left is -1.0 and full right is 1.0
	Reserved uint16 // 16 bits,
}

// Box Types: hmhd
// The hint media header contains general information, independent of the protocol, for hint tracks.
// (A PDU is a Protocol Data Unit.)
type HintMediaHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	MaxPDUSize uint16 // 16 bits, gives the size in bytes of the largest PDU in this (hint) stream
	AvgPDUSize uint16 // 16 bits, gives the average size of a PDU over the entire presentation
	MaxBitrate uint32 // 32 bits, gives the maximum rate in bits/second over any window of one second
	AvgBitrate uint32 // 32 bits, gives the average rate in bits/second over the entire presentation
	Reserved   uint32 // 32 bits,
}

// Box Types: nmhd
// Streams other than visual and audio (e.g., timed metadata streams) may use a null Media Header Box, as defined here.
type NullMediaHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/56
//
// Box Type: dinf
// Container: Media Information Box ('minf') or Meta Box ('meta')
// Mandatory: Yes (required within 'minf' box) and No (optional within 'meta' box)
// Quantity: Exactly one
//
// The data information box contains objects that declare the location of the media information in a track
type DataInformationBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------
//
// ISO_IEC_14496-12_2012.pdf Page/56
//
// Box Types: url, urn, dref
// Container: Data Information Box ('dinf')
// Mandatory: Yes
// Quantity: Exactly one
type DataReferenceBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint32      // 32 bits, is an integer that gives the number of entries in the following table
	DataEntry  interface{} // DataEntryUrlBox or DataEntryUrnBox.
}

// aligned(8) class DataReferenceBox
// 	extends FullBox('dref', version = 0, 0) {
// 	unsigned int(32) entry_count;
// 	for (i=1; i <= entry_count; i++) {
// 		DataEntryBox(entry_version, entry_flags) data_entry;
// 	}
// }

type DataEntryUrlBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Location string // string,
}

type DataEntryUrnBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Name     string // string,
	Location string // string,
}

// -------------------------------------------------------------------------------------------------------
//
// ISO_IEC_14496-12_2012.pdf Page/40
//
// Box Type: stbl
// Container: Media Information Box ('minf')
// Mandatory: Yes
// Quantity: Exactly one
type SampleTableBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/40
//
// Box Types: stsd
// Container: Sample Table Box ('stbl')
// Mandatory: Yes
// Quantity: Exactly one
type SampleDescriptionBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint32 // 32 bits, is an integer that gives the number of entries in the following table
}

// for (i = 1 ; i <= entry_count ; i++) {
// 	switch (handler_type){
// 		case ‘soun’: // for audio tracks
// 			AudioSampleEntry();
// 			break;
// 		case ‘vide’: // for video tracks
// 			VisualSampleEntry();
// 			break;
// 		case ‘hint’: // Hint track
// 			HintSampleEntry();
// 			break;
// 		case ‘meta’: // Metadata track
// 			MetadataSampleEntry();
// 			break;
// 	}
// }

// box header和version字段后会有一个entry count字段,根据entry的个数,每个entry会有type信息,如“vide”、“sund”等,
// 根据type不同sample description会提供不同的信息,例如对于video track,会有“VisualSampleEntry”类型信息,
// 对于audio track会有“AudioSampleEntry”类型信息.
// 视频的编码类型、宽高、长度,音频的声道、采样等信息都会出现在这个box中

// is the appropriate sample entry
type SampleEntry struct {
	Reserved           [6]uint8 // 48 bits,
	DataReferenceIndex uint16   // 16 bits, is an integer that contains the index of the data reference to use to retrieve data associated with samples that use this sample description. Data references are stored in Data Reference Boxes. The index ranges from 1 to the number of data references.
}

type HintSampleEntry struct {
	Data []uint8 // 8 bits array,
}

// Box Types: btrt
type BitRateBox struct {
	MP4BoxHeader // standard header

	BufferSizeDB uint32 // 32 bits, gives the size of the decoding buffer for the elementary stream in bytes.
	MaxBitrate   uint32 // 32 bits, gives the maximum rate in bits/second over any window of one second.
	AvgBitrate   uint32 // 32 bits, gives the average rate in bits/second over the entire presentation.
}

type MetaDataSampleEntry struct{}

type XMLMetaDataSampleEntry struct {
	ContentEncoding string     // optional, is a null-terminated string in UTF-8 characters, and provides a MIME type which identifies the content encoding of the timed metadata
	NameSpace       string     // string, gives the namespace of the schema for the timed XML metadata
	SchemaLocation  string     // optional, optionally provides an URL to find the schema corresponding to the namespace. This is needed for decoding of the timed metadata by XML aware encoding mechanisms such as BiM.
	Brb             BitRateBox // optional
}

type TextMetaDataSampleEntry struct {
	ContentEncoding string     // optional, is a null-terminated string in UTF-8 characters, and provides a MIME type which identifies the content encoding of the timed metadata
	MimeFormat      string     // string, provides a MIME type which identifies the content format of the timed metadata. Examples for this field are ‘text/html’ and ‘text/plain’.
	Brb             BitRateBox // optional
}

type URIBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	TheURI string // string, is a URI formatted according to the rules in 6.2.4
}

type URIInitBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	UriInitializationData []uint8 // 8 bits array,  is opaque data whose form is defined in the documentation of the URI form.
}

type URIMetaSampleEntry struct {
	TheLabel URIBox
	Init     URIInitBox // optional
	//Mpeg4    MPEG4BitRateBox // optional
}

// Box Types: pasp
type PixelAspectRatioBox struct {
	MP4BoxHeader // standard header

	HSpacing uint32 // 32 bits, define the relative width and height of a pixel;
	VSpacing uint32 // 32 bits, define the relative width and height of a pixel;
}

// Box Types: clap
// Visual Sequences
type CleanApertureBox struct {
	MP4BoxHeader // standard header

	CleanApertureWidthN  uint32 // 32 bits, a fractional number which defines the exact clean aperture width, in counted pixels, of the video image
	CleanApertureWidthD  uint32 // 32 bits, a fractional number which defines the exact clean aperture width, in counted pixels, of the video image
	CleanApertureHeightN uint32 // 32 bits, a fractional number which defines the exact clean aperture height, in counted pixels, of the video image
	CleanApertureHeightD uint32 // 32 bits, a fractional number which defines the exact clean aperture height, in counted pixels, of the video image
	HorizOffN            uint32 // 32 bits, a fractional number which defines the horizontal offset of clean aperture centre minus (width-1)/2. Typically 0
	HorizOffD            uint32 // 32 bits, a fractional number which defines the horizontal offset of clean aperture centre minus (width-1)/2. Typically 0
	VertOffN             uint32 // 32 bits, a fractional number which defines the vertical offset of clean aperture centre minus (height-1)/2. Typically 0
	VertOffD             uint32 // 32 bits, a fractional number which defines the vertical offset of clean aperture centre minus (height-1)/2. Typically 0
}

// Box Types: colr
type ColourInformationBox struct {
	MP4BoxHeader // standard header

	ColourType uint32 // 32 bits, an indication of the type of colour information supplied. For colour_type ‘nclx’: these fields are exactly the four bytes defined for PTM_COLOR_INFO( ) in A.7.2 of ISO/IEC 29199-2 but note that the full range flag is here in a different bit position
}

// if (colour_type == ‘nclx’) /* on-screen colours */
// {
// 	unsigned int(16) colour_primaries;
// 	unsigned int(16) transfer_characteristics;
// 	unsigned int(16) matrix_coefficients;
// 	unsigned int(1) full_range_flag;
// 	unsigned int(7) reserved = 0;
// }
// else if (colour_type == ‘rICC’)
// {
// 	ICC_profile; // restricted ICC profile
// }
// else if (colour_type == ‘prof’)
// {
// 	ICC_profile; // unrestricted ICC profile
// }

// ICC_profile : an ICC profile as defined in ISO 15076-1 or ICC.1:2010 is supplied.

type VisualSampleEntry struct {
	PreDefined1     uint16              // 16 bits,
	Reserved1       uint16              // 16 bits,
	PreDefined2     [3]uint32           // 96 bits,
	Width           uint16              // 16 bits, are the maximum visual width and height of the stream described by this sample description, in pixels
	Height          uint16              // 16 bits, are the maximum visual width and height of the stream described by this sample description, in pixels
	HorizreSolution uint32              // 32 bits, fields give the resolution of the image in pixels-per-inch, as a fixed 16.16 number
	VertreSolution  uint32              // 32 bits, fields give the resolution of the image in pixels-per-inch, as a fixed 16.16 number
	Reserved3       uint32              // 32 bits,
	FrameCount      uint16              // 16 bits, indicates how many frames of compressed video are stored in each sample. The default is 1, for one frame per sample; it may be more than 1 for multiple frames per sample
	CompressorName  [32]string          // 32 string, is a name, for informative purposes. It is formatted in a fixed 32-byte field, with the first byte set to the number of bytes to be displayed, followed by that number of bytes of displayable data, and then padding to complete 32 bytes total (including the size byte). The field may be set to 0.
	Depth           uint16              // 16 bits, takes one of the following values 0x0018 – images are in colour with no alpha
	PreDefined3     int16               // 16 bits,
	Cab             CleanApertureBox    // optional, other boxes from derived specifications
	Parb            PixelAspectRatioBox // optional, other boxes from derived specifications
}

// Audio Sequences
type AudioSampleEntry struct {
	Reserved1    [2]uint32 // 32 bits array,
	ChannelCount uint16    // 16 bits, is the number of channels such as 1 (mono) or 2 (stereo)
	SampleSize   uint16    // 16 bits, is in bits, and takes the default value of 16
	PreDefined   uint16    // 16 bits,
	Reserved2    uint16    // 16 bits,
	SampleRate   uint32    // 32 bits, is the sampling rate expressed as a 16.16 fixed-point number (hi.lo)
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/48
//
// Box Type: stts
// Container: Sample Table Box ('stbl')
// Mandatory: Yes
// Quantity: Exactly one
type TimeToSampleBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint32              // 32 bits, is an integer that gives the number of entries in the following table
	Table      []TimeToSampleTable // Time To Sample Table , EntryCount elements
}

type TimeToSampleTable struct {
	SampleCount []uint32 // 32 bits, is an integer that counts the number of consecutive samples that have the given duration
	SampleDelta []uint32 // 32 bits, is an integer that gives the delta of these samples in the time-scale of the media.
}

// “stts”存储了sample的duration,描述了sample时序的映射方法,我们通过它可以找到任何时间的sample.
// “stts”可以包含一个压缩的表来映射时间和sample序号,用其他的表来提供每个sample的长度和指针.
// 表中每个条目提供了在同一个时间偏移量里面连续的sample序号,以及samples的偏移量.
// 递增这些偏移量,就可以建立一个完整的time to sample表

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/49
//
// Box Type: ctts
// Container: Sample Table Box (‘stbl’)
// Mandatory: No
// Quantity: Zero or one
type CompositionOffsetBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint32                   // 32 bits, is an integer that gives the number of entries in the following table
	Table      []CompositionOffsetTable // Composition Offset Table, EntryCount elements.
}

type CompositionOffsetTable struct {
	SampleCount  uint32      // 32 bits, is an integer that counts the number of consecutive samples that have the given offset.
	SampleOffset interface{} // int32 or uint32, is an integer that gives the offset between CT and DT, such that CT(n) = DT(n) + CTTS(n).
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/50
//
// Box Type: cslg
// Container: Sample Table Box (‘stbl’)
// Mandatory: No
// Quantity: Zero or one
type CompositionToDecodeBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	CompositionToDTSShift        int32 // 32 bits, signed, if this value is added to the composition times (as calculated by the CTS offsets from the DTS), then for all samples, their CTS is guaranteed to be greater than or equal to their DTS, and the buffer model implied by the indicated profile/level will be honoured; if leastDecodeToDisplayDelta is positive or zero, this field can be 0; otherwise it should be at least (- leastDecodeToDisplayDelta)
	LeastDecodeToDisplayDelta    int32 // 32 bits, signed, the smallest composition offset in the CompositionTimeToSample box in this track
	GreatestDecodeToDisplayDelta int32 // 32 bits, signed, the largest composition offset in the CompositionTimeToSample box in this track
	CompositionStartTime         int32 // 32 bits, signed, the smallest computed composition time (CTS) for any sample in the media of this track
	CompositionEndTime           int32 // 32 bits, signed, the composition time plus the composition duration, of the sample with the largest computed composition time (CTS) in the media of this track; if this field takes the value 0, the composition end time is unknown.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/58
//
// Box Type: stsc
// Container: Sample Table Box ('stbl')
// Mandatory: Yes
// Quantity: Exactly one
type SampleToChunkBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint32               // 32 bits, is an integer that gives the number of entries in the following table
	Table      []SampleToChunkTable // Sample To Chunk Table, entry count elements.
}

type SampleToChunkTable struct {
	FirstChunk             []uint32 // 32 bits, is an integer that gives the index of the first chunk in this run of chunks that share the same samples-per-chunk and sample-description-index; the index of the first chunk in a track has the value 1 (the first_chunk field in the first record of this box has the value 1, identifying that the first sample maps to the first chunk).
	SamplesPerChunk        []uint32 // 32 bits, is an integer that gives the number of samples in each of these chunks
	SampleDescriptionIndex []uint32 // 32 bits, is an integer that gives the index of the sample entry that describes the samples in this chunk. The index ranges from 1 to the number of sample entries in the Sample Description Box
}

// 用chunk组织sample可以方便优化数据获取,一个thunk包含一个或多个sample.
// “stsc”中用一个表描述了sample与chunk的映射关系,查看这张表就可以找到包含指定sample的thunk,从而找到这个sample

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/57
//
// Box Type: stsz, stz2
// Container: Sample Table Box (‘stbl’)
// Mandatory: Yes
// Quantity: Exactly one variant must be present
type SampleSizeBoxes struct{}

// Box Type: stsz
type SampleSizeBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SampleSize  uint32      // 32 bits, is integer specifying the default sample size. If all the samples are the same size, this field contains that size value. If this field is set to 0, then the samples have different sizes, and those sizes are stored in the sample size table. If this field is not 0, it specifies the constant sample size, and no array follows.
	SampleCount uint32      // 32 bits, is an integer that gives the number of samples in the track; if sample-size is 0, then it is also the number of entries in the following table.
	EntrySize   interface{} // 32 bits array, SampleCount elements, is an integer specifying the size of a sample, indexed by its number.
}

// if (sample_size	==	0) {
// 	for (i = 1; i <= sample_count; i++) {
// 		unsigned int(32) entry_size;
// 	}
// }

// Box Type: stz2
type CompactSampleSizeBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Reserved    [3]uint8    // 24 bits,
	FieldSize   uint8       // 8 bits, is an integer specifying the size in bits of the entries in the following table; it shall take the value 4, 8 or 16. If the value 4 is used, then each byte contains two values: entry[i]<<4 + entry[i+1]; if the sizes do not fill an integral number of bytes, the last byte is padded with zeros.
	SampleCount uint32      // 32 bits,  is an integer that gives the number of entries in the following table
	EntrySize   interface{} //
}

// for (i = 1; i <= sample_count; i++) {
// 	unsigned int(field_size) entry_size;
// }

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/59
//
// Box Type: stco, co64
// Container: Sample Table Box (‘stbl’)
// Mandatory: Yes
// Quantity: Exactly one variant must be present
type ChunkOffsetBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount  uint32   // 32 bits, is an integer that gives the number of entries in the following table
	ChunkOffset []uint32 // 32 bits array, entry count elements.
}

// “stco”定义了每个thunk在媒体流中的位置。位置有两种可能，32位的和64位的，后者对非常大的电影很有用。
// 在一个表中只会有一种可能，这个位置是在整个文件中的，而不是在任何box中的，这样做就可以直接在文件中找到媒体数据，
// 而不用解释box。需要注意的是一旦前面的box有了任何改变，这张表都要重新建立，因为位置信息已经改变了

// Box Type: co64
type ChunkLargeOffsetBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount  uint32   // 32 bits, is an integer that gives the number of entries in the following table
	ChunkOffset []uint64 // 64 bits array, entry count elements.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/51
//
// Box Type: stss
// Container: Sample Table Box (‘stbl’)
// Mandatory: No
// Quantity: Zero or one
//
// This box provides a compact marking of the sync samples within the stream. The table is arranged in strictly increasing order of sample number.
// If the sync sample box is not present, every sample is a sync sample.
type SyncSampleBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount   uint32   // 32 bits, is an integer that gives the number of entries in the following table. If entry_count is zero, there are no sync samples within the stream and the following table is empty
	SampleNumber []uint32 // 32 bits array, entry count elements. gives the numbers of the samples that are sync samples in the stream.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/52
//
// Box Type: stsh
// Container: Sample Table Box (‘stbl’)
// Mandatory: No
// Quantity: Zero or one
type ShadowSyncSampleBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint32                  // 32 bits, is an integer that gives the number of entries in the following table.
	Table      []ShadowSyncSampleTable // Shadow Sync Sample Table, entry count elements.
}

type ShadowSyncSampleTable struct {
	ShadowedSampleNumber uint32 // 32 bits, gives the number of a sample for which there is an alternative sync sample.
	SyncSampleNumber     uint32 // 32 bits, gives the number of the alternative sync sample.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/60
//
// Box Type: padb
// Container: Sample Table (‘stbl’)
// Mandatory: No
// Quantity: Zero or one
//
// In some streams the media samples do not occupy all bits of the bytes given by the sample size, and are
// padded at the end to a byte boundary. In some cases, it is necessary to record externally the number of
// padding bits used. This table supplies that information.
type PaddingBitsBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SampleCount uint32             // 32 bits, counts the number of samples in the track; it should match the count in other tables
	Table       []PaddingBitsTable // Padding Bits Table, (sample count + 1) / 2 elements.
}

type PaddingBitsTable struct {
	Reserved1 byte // 1 bit,
	Pad1      byte // 3 bits, a value from 0 to 7, indicating the number of bits at the end of sample (i*2)+1.
	Reserved2 byte // 1 bit,
	Pad2      byte // 3 bits, a value from 0 to 7, indicating the number of bits at the end of sample (i*2)+2.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/46
//
// Box Type: stdp
// Container: Sample Table Box (‘stbl’).
// Mandatory: No.
// Quantity: Zero or one.
//
// This box contains the degradation priority of each sample. The values are stored in the table, one for each
// sample. The size of the table, sample_count is taken from the sample_count in the Sample Size Box
// ('stsz'). Specifications derived from this define the exact meaning and acceptable range of the priority field.
type DegradationPriorityBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Priority []uint16 // 16 bits array, sample count elements, is integer specifying the degradation priority for each sample.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/53
//
// Box Types: sdtp
// Container: Sample Table Box (‘stbl’)
// Mandatory: No
// Quantity: Zero or one
type IndependentAndDisposableSamplesBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Table []IndependentAndDisposableSamplesTable // Independent And Disposable Samples Table, sample count elements
}

type IndependentAndDisposableSamplesTable struct {
	IsLeading           byte // 2 bits,
	SampleDependsOn     byte // 2 bits,
	SampleIsDependedOn  byte // 2 bits,
	SampleHasTedundancy byte // 2 bits,
}

// is_leading takes one of the following four values:
// 0: the leading nature of this sample is unknown;
// 1: this sample is a leading sample that has a dependency before the referenced I-picture (and is
// therefore not decodable);
// 2: this sample is not a leading sample;
// 3: this sample is a leading sample that has no dependency before the referenced I-picture (and is
// therefore decodable);
// sample_depends_on takes one of the following four values:
// 0: the dependency of this sample is unknown;
// 1: this sample does depend on others (not an I picture);
// 2: this sample does not depend on others (I picture);
// 3: reserved
// sample_is_depended_on takes one of the following four values:
// 0: the dependency of other samples on this sample is unknown;
// 1: other samples may depend on this one (not disposable);
// 2: no other sample depends on this one (disposable);
// 3: reserved
// sample_has_redundancy takes one of the following four values:
// 0: it is unknown whether there is redundant coding in this sample;
// 1: there is redundant coding in this sample;
// 2: there is no redundant coding in this sample;
// 3: reserved

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/75
//
// Box Type: sbgp
// Container: Sample Table Box (‘stbl’) or Track Fragment Box (‘traf’)
// Mandatory: No
// Quantity: Zero or more.
type SampleToGroupBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	GroupingType uint32               // 32 bits, is an integer that identifies the type (i.e. criterion used to form the sample groups) of the sample grouping and links it to its sample group description table with the same value for grouping type. At most one occurrence of this box with the same value for grouping_type (and, if used, grouping_type_parameter) shall exist for a track.
	EntryCount   uint32               // 32 bits, is an integer that gives the number of entries in the following table.
	Table        []SampleToGroupTable // Sample To Group Table, entry count elements.
}

type SampleToGroupTable struct {
	SampleCount           uint32 // 32 bits, is an integer that gives the number of consecutive samples with the same sample group descriptor. If the sum of the sample count in this box is less than the total sample count, then the reader should effectively extend it with an entry that associates the remaining samples with no group. It is an error for the total in this box to be greater than the sample_count documented elsewhere, and the reader behaviour would then be undefined.
	GroupDescriptionIndex uint32 // 32 bits, is an integer that gives the index of the sample group entry which describes the samples in this group. The index ranges from 1 to the number of sample group entries in the SampleGroupDescription Box, or takes the value 0 to indicate that this sample is a member of no group of this type.
}

// unsigned int(32) grouping_type;
// if (version == 1) {
// unsigned int(32) grouping_type_parameter;
// }
// unsigned int(32) entry_count;

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/76
//
// Box Type: sgpd
// Container: Sample Table Box (‘stbl’) or Track Fragment Box (‘traf’)
// Mandatory: No
// Quantity: Zero or more, with one for each Sample to Group Box.
type SampleGroupDescriptionBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	GroupingType uint32 // 32 bits, is an integer that identifies the SampleToGroup box that is associated with this sample group description.
	EntryCount   uint32 // 32 bits, is an integer that gives the number of entries in the following table.
}

// default_length : indicates the length of every group entry (if the length is constant), or zero (0) if it is variable
// description_length : indicates the length of an individual group entry, in the case it varies from entry to entry and default_length is therefore 0

// if (version==1) { unsigned int(32) default_length; }

// for (i = 1 ; i <= entry_count ; i++){
// if (version==1) {
// if (default_length==0) {
// unsigned int(32) description_length;
// }
// }
// switch (handler_type){
// case ‘vide’: // for video tracks
// VisualSampleGroupEntry (grouping_type);
// break;
// case ‘soun’: // for audio tracks
// AudioSampleGroupEntry(grouping_type);
// break;
// case ‘hint’: // for hint tracks
// HintSampleGroupEntry(grouping_type);
// break;
// }
// }

type SampleGroupDescriptionEntry struct{}

type VisualSampleGroupEntry struct{}

type AudioSampleGroupEntry struct{}

type HintSampleGroupEntry struct{}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/61
//
// Box Type: subs
// Container: Sample Table Box (‘stbl’) or Track Fragment Box (‘traf’)
// Mandatory: No
// Quantity: Zero or one
type SubSampleInformationBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint32                      // 32 bits, is an integer that gives the number of entries in the following table.
	Table      []SubSampleInformationTable // Sub-Sample Information Table, entry count elements.

}

type SubSampleInformationTable struct {
	SampleDelta    uint32                // 32 bits, is an integer that specifies the sample number of the sample having sub-sample structure. It is coded as the difference between the desired sample number, and the sample number indicated in the previous entry. If the current entry is the first entry, the value indicates the sample number of the first sample having sub-sample information, that is, the value is the difference between the sample number and zero (0).
	SubsampleCount uint16                // 16 bits, is an integer that specifies the number of sub-sample for the current sample. If there is no sub-sample structure, then this field takes the value 0.
	CountTable     []SubSampleCountTable // Sub-Sample Information Table1, subsample count elements.
}

type SubSampleCountTable struct {
	SubsampleSize     interface{} // uint16 or uint32, is an integer that specifies the size, in bytes, of the current sub-sample
	SubsamplePriority uint8       // 8 bits, is an integer specifying the degradation priority for each sub-sample. Higher values of subsample_priority, indicate sub-samples which are important to, and have a greater impact on, the decoded quality.
	DiscardAble       uint8       // 8 bits, equal to 0 means that the sub-sample is required to decode the current sample, while equal to 1 means the sub-sample is not required to decode the current sample but may be used for enhancements, e.g., the sub-sample consists of supplemental enhancement information (SEI) messages.
	Reserved          uint32      // 32 bits,
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/62
//
// Box Type: saiz
// Container: Sample Table Box (‘stbl’) or Track Fragment Box ('traf')
// Mandatory: No
// Quantity: Zero or More
type SampleAuxiliaryInformationSizesBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Table interface{} // SampleAuxiliaryInformationSizesTable1 or SampleAuxiliaryInformationSizesTable2.
}

type SampleAuxiliaryInformationSizesTable1 struct {
	AuxInfoType           uint32 // 32 bits,
	AuxInfoTypeParameter  uint32 // 32 bits,
	DefaultSampleInfoSize uint8  // 8 bits,  is an integer specifying the sample auxiliary information size for the case where all the indicated samples have the same sample auxiliary information size. If the size varies then this field shall be zero.
	SampleCount           uint32 // 32 bits,
}

type SampleAuxiliaryInformationSizesTable2 struct {
	DefaultSampleInfoSize uint8  // 8 bits,  is an integer specifying the sample auxiliary information size for the case where all the indicated samples have the same sample auxiliary information size. If the size varies then this field shall be zero.
	SampleCount           uint32 // 32 bits,
}

// if (flags & 1) {
// unsigned int(32) aux_info_type;
// unsigned int(32) aux_info_type_parameter;
// }
// unsigned int(8) default_sample_info_size;
// unsigned int(32) sample_count;
// if (default_sample_info_size == 0) {
// unsigned int(8) sample_info_size[ sample_count ];
// }

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/63
//
// Box Type: saio
// Container: Sample Table Box (‘stbl’) or Track Fragment Box ('traf')
// Mandatory: No
// Quantity: Zero or More
type SampleAuxiliaryInformationOffsetsBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	//EntryCount uint32 // 32 bits, is an integer that gives the number of entries in the following table.
}

type AuxInfo struct {
	AuxInfoType          uint32 // 32 bits,
	AuxInfoTypeParameter uint32 // 32 bits,
}

// if (flags & 1) {
// unsigned int(32) aux_info_type;
// unsigned int(32) aux_info_type_parameter;
// }
// unsigned int(32) entry_count;
// if ( version == 0 ) {
// unsigned int(32) offset[ entry_count ];
// }
// else {
// unsigned int(64) offset[ entry_count ];
// }

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/78
//
// Box Type: udta
// Container: Movie Box (‘moov’) or Track Box (‘trak’)
// Mandatory: No
// Quantity: Zero or one
type UserDataBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/64
//
// Box Type: mvex
// Container: Movie Box (‘moov’)
// Mandatory: No
// Quantity: Zero or one
type MovieExtendsBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/65
//
// Box Type: mehd
// Container: Movie Extends Box(‘mvex’)
// Mandatory: No
// Quantity: Zero or one
//
// The Movie Extends Header is optional, and provides the overall duration, including fragments, of a fragmented
// movie. If this box is not present, the overall duration must be computed by examining each fragment.
type MovieExtendsHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header
}

// if (version==1) {
// unsigned int(64) fragment_duration;
// } else { // version==0
// unsigned int(32) fragment_duration;
// }

// fragment_duration : is an integer that declares length of the presentation of the whole movie including
// fragments (in the timescale indicated in the Movie Header Box). The value of this field corresponds to
// the duration of the longest track, including movie fragments. If an MP4 file is created in real-time, such
// as used in live streaming, it is not likely that the fragment_duration is known in advance and this
// box may be omitted.

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/65
//
// Box Type: trex
// Container: Movie Extends Box (‘mvex’)
// Mandatory: Yes
// Quantity: Exactly one for each track in the Movie Box
type TrackExtendsBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	TrackID                       uint32 // 32 bits, identifies the track; this shall be the track ID of a track in the Movie Box
	DefaultSampleDescriptionIndex uint32 // 32 bits,
	DefaultSampleDuration         uint32 // 32 bits,
	DefaultSampleSize             uint32 // 32 bits,
	DefaultSampleFlags            uint32 // 32 bits,
}

// default_ : these fields set up defaults used in the track fragments.

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/72
//
// Box Type: leva
// Container: Movie Extends Box (`mvex’)
// Mandatory: No
// Quantity: Zero or one
type LevelAssignmentBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	LevelCount uint8                  // 8 bits, specifies the number of levels each fraction is grouped into. level_count shall be greater than or equal to 2.
	Table      []LevelAssignmentTable // Level Assignment Table, level count elements.
}

type LevelAssignmentTable struct {
	TrackId        uint32 // 32 bits, for loop entry j specifies the track identifier of the track assigned to level j.
	PaddingFlag    byte   // 1 bit, equal to 1 indicates that a conforming fraction can be formed by concatenating any positive integer number of levels within a fraction and padding the last Media Data box by zero bytes up to the full size that is indicated in the header of the last Media Data box. The semantics of padding_flag equal to 0 are that this is not assured.
	AssignmentType byte   // 7 bits,
}

// for (j=1; j <= level_count; j++) {
// unsigned int(32) track_id;
// unsigned int(1) padding_flag;
// unsigned int(7) assignment_type;
// if (assignment_type == 0) {
// unsigned int(32) grouping_type;
// }
// else if (assignment_type == 1) {
// unsigned int(32) grouping_type;
// unsigned int(32) grouping_type_parameter;
// }
// else if (assignment_type == 2) {} // no further syntax elements needed
// else if (assignment_type == 3) {} // no further syntax elements needed
// else if (assignment_type == 4) {
// unsigned int(32) sub_track_id;
// }
// // other assignment_type values are reserved
// }

// assignment_type : indicates the mechanism used to specify the assignment to a level.
// assignment_type values greater than 4 are reserved, while the semantics for the other values are
// specified as follows. The sequence of assignment_types is restricted to be a set of zero or more of
// type 2 or 3, followed by zero or more of exactly one type.
// • 0: sample groups are used to specify levels, i.e., samples mapped to different sample group
// description indexes of a particular sample grouping lie in different levels within the identified track;
// other tracks are not affected and must have all their data in precisely one level;
// • 1: as for assignment_type 0 except assignment is by a parameterized sample group;
// • 2, 3: level assignment is by track (see the Subsegment Index Box for the difference in processing
// of these levels)
// • 4: the respective level contains the samples for a sub-track. The sub-tracks are specified through
// the Sub Track box; other tracks are not affected and must have all their data in precisely one
// level;

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/66
//
// Box Type: moof
// Container: File
// Mandatory: No
// Quantity: Zero or more
type MovieFragmentBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/66
//
// Box Type: mfhd
// Container: Movie Fragment Box ('moof')
// Mandatory: Yes
// Quantity: Exactly one
//
// The movie fragment header contains a sequence number, as a safety check. The sequence number usually
// starts at 1 and must increase for each movie fragment in the file, in the order in which they occur. This allows
// readers to verify integrity of the sequence; it is an error to construct a file where the fragments are out of sequence.
type MovieFragmentHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SequenceNumber uint32 // 32 bits, the ordinal number of this fragment, in increasing order
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/67
//
// Box Type: traf
// Container: Movie Fragment Box ('moof')
// Mandatory: No
// Quantity: Zero or more
type TrackFragmentBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/67
//
// Box Type: tfhd
// Container: Track Fragment Box ('traf')
// Mandatory: Yes
// Quantity: Exactly one
type TrackFragmentHeaderBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	TrackID uint32 // 32 bits,

	// all the following are optional fields
	BaseDataOffset         uint64 // 64 bits, the base offset to use when calculating data offsets
	SampleDescriptionIndex uint32 // 32 bits,
	DefaultSampleDuration  uint32 // 32 bits,
	DefaultSampleSize      uint32 // 32 bits,
	DefaultSampleFlags     uint32 // 32 bits,
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/68
//
// Box Type: trun
// Container: Track Fragment Box ('traf')
// Mandatory: No
// Quantity: Zero or more
type TrackFragmentRunBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SampleCount uint32 // 32 bits, the number of samples being added in this run; also the number of rows in the following table (the rows can be empty)

	// the following are optional fields
	DataOffset       int32  // 32 bits, signed, is added to the implicit or explicit data_offset established in the track fragment header.
	FirstSampleFlags uint32 // 32 bits, provides a set of flags for the first sample only of this run.

	// all fields in the following array are optional
	Table []TrackFragmentRunTable // Track Fragment Run Table 1, SampleCount elements.
}

type TrackFragmentRunTable struct {
	SampleDuration              uint32      // 32 bits,
	SampleSize                  uint32      // 32 bits,
	SampleFlags                 uint32      // 32 bits,
	SampleCompositionTimeOffset interface{} // uint32 or int32,
}

// if (version == 0){
// 	unsigned int(32) sample_composition_time_offset;
// }
// else{
// 	signed int(32) sample_composition_time_offset;
// }

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/71
//
// Box Type: tfdt
// Container: Track Fragment box (‘traf’)
// Mandatory: No
// Quantity: Zero or one
type TrackFragmentBaseMediaDecodeTimeBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	BaseMediaDecodeTime interface{} // uint32 or uint64, is an integer equal to the sum of the decode durations of all earlier samples in the media, expressed in the media's timescale. It does not include the samples added in the enclosing track fragment.
}

// if (version==1) {
// unsigned int(64) baseMediaDecodeTime;
// } else { // version==0
// unsigned int(32) baseMediaDecodeTime;

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/69
//
// Box Type: mfra
// Container: File
// Mandatory: No
// Quantity: Zero or one
type MovieFragmentRandomAccessBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/70
//
// Box Type: tfra
// Container: Movie Fragment Random Access Box (‘mfra’)
// Mandatory: No
// Quantity: Zero or one per track
type TrackFragmentRandomAccessBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	TrackID               uint32                           // 32 bits, is an integer identifying the track_ID.
	Reserved              uint32                           // 26 bits,
	LengthSizeOfTrafNum   byte                             // 2 bits, indicates the length in byte of the traf_number field minus one.
	LengthSizeOfTrunNum   byte                             // 2 bits, indicates the length in byte of the trun_number field minus one.
	LengthSizeOfSampleNum byte                             // 2 bits, indicates the length in byte of the sample_number field minus one.
	NumberOfEntry         uint32                           // 32 bits, is an integer that gives the number of the entries for this track. If this value is zero, it indicates that every sample is a sync sample and no table entry follows.
	Table                 []TrackFragmentRandomAccessTable // Track Fragment RandomAccess Table 1, NumberOfEntry elements.
}

type TrackFragmentRandomAccessTable struct {
	Time         interface{} // uint32 or uint64, is 32 or 64 bits integer that indicates the presentation time of the sync sample in units defined in the ‘mdhd’ of the associated track.
	Moofoffset   interface{} // uint32 or uint64, is 32 or 64 bits integer that gives the offset of the ‘moof’ used in this entry. Offset is the byte-offset between the beginning of the file and the beginning of the ‘moof’.
	TrafNumber   interface{} // unsigned int((length_size_of_traf_num+1) * 8). indicates the ‘traf’ number that contains the sync sample. The number ranges from 1 (the first ‘traf’ is numbered 1) in each ‘moof’.
	TrunNumber   interface{} // unsigned int((length_size_of_trun_num+1) * 8). indicates the ‘trun’ number that contains the sync sample. The number ranges from 1 in each ‘traf’
	SampleNumber interface{} // unsigned int((length_size_of_sample_num+1) * 8) . indicates the sample number of the sync sample. The number ranges from 1 in each ‘trun’.
}

// for(i=1; i <= number_of_entry; i++){
// if(version==1){
// unsigned int(64) time;
// unsigned int(64) moof_offset;
// }else{
// unsigned int(32) time;
// unsigned int(32) moof_offset;
// }
// unsigned int((length_size_of_traf_num+1) * 8) traf_number;
// unsigned int((length_size_of_trun_num+1) * 8) trun_number;
// unsigned int((length_size_of_sample_num+1) * 8) sample_number;
// }

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/71
//
// Box Type: mfro
// Container: Movie Fragment Random Access Box (‘mfra’)
// Mandatory: Yes
// Quantity: Exactly one
//
// The Movie Fragment Random Access Offset Box provides a copy of the length field from the enclosing Movie
// Fragment Random Access Box. It is placed last within that box, so that the size field is also last in the
// enclosing Movie Fragment Random Access Box. When the Movie Fragment Random Access Box is also last
// in the file this permits its easy location. The size field here must be correct. However, neither the presence of
// the Movie Fragment Random Access Box, nor its placement last in the file, are assured.
type MovieFragmentRandomAccessOffsetBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Size uint32 // 32 bits, is an integer gives the number of bytes of the enclosing ‘mfra’ box. This field is placed at the last of the enclosing box to assist readers scanning from the end of the file in finding the ‘mfra’ box.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/29
//
// Box Type: mdat
// Container: File
// Mandatory: No
// Quantity: Zero or more
type MediaDataBox struct {
	MP4BoxHeader // standard header

	Data []byte // 8 bits array, is the contained media data.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/29
//
// Box Types: free, skip
// Container: File or other box
// Mandatory: No
// Quantity: Zero or more
//
// The contents of a free-space box are irrelevant and may be ignored, or the object deleted, without affecting
// the presentation. (Care should be exercised when deleting the object, as this may invalidate the offsets used
// in the sample table, unless this object is after all the media data).
type FreeSpaceBox struct {
	MP4BoxHeader // standard header

	Data []uint8 // 8 bits array,
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/78
//
// Box Type: cprt
// Container: User data box (‘udta’)
// Mandatory: No
// Quantity: Zero or more
//
// The Copyright box contains a copyright declaration which applies to the entire presentation, when contained
// within the Movie Box, or, when contained in a track, to that entire track. There may be multiple copyright
// boxes using different language codes.
type CopyrightBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Pad      byte    // 1 bit,
	Language [2]byte // 15 bits, declares the language code for the following text. See ISO 639-2/T for the set of three character codes. Each character is packed as the difference between its ASCII value and 0x60. The code is confined to being three lower-case letters, so these values are strictly positive.
	Notice   string  // string, is a null-terminated string in either UTF-8 or UTF-16 characters, giving a copyright notice. If UTF- 16 is used, the string shall start with the BYTE ORDER MARK (0xFEFF), to distinguish it from a UTF- 8 string. This mark does not form part of the final string.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/79
//
// Box Type: tsel
// Container: User Data Box (‘udta’)
// Mandatory: No
// Quantity: Zero or One
//
// The track selection box is contained in the user data box of the track it modifies.
type TrackSelectionBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SwitchGroup   int32    // 32 bits, is an integer that specifies a group or collection of tracks. If this field is 0 (default value) or if the Track Selection box is absent there is no information on whether the track can be used for switching during playing or streaming. If this integer is not 0 it shall be the same for tracks that can be used for switching between each other. Tracks that belong to the same switch group shall belong to the same alternate group. A switch group may have only one member.
	AttributeList []uint32 // 32 bits array, to end of the box, is a list, to the end of the box, of attributes. The attributes in this list should be used as descriptions of tracks or differentiation criteria for tracks in the same alternate or switch group. Each differentiating attribute is associated with a pointer to the field or information that distinguishes the track.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/100
//
// Box Type: strk
// Container: User Data box (‘udta’) of the corresponding Track box (‘trak’)
// Mandatory: No
// Quantity: Zero or more
//
// This box contains objects that define and provide information about a sub track in the present track.
type SubTrack struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/100
//
// Box Type: stri
// Container: Sub Track box (‘strk’)
// Mandatory: Yes
// Quantity: One
type SubTrackInformation struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SwitchGroup    int16    // 16 bits,
	AlternateGroup int16    // 16 bits,
	SubTrackID     uint32   // 32 bits, is an integer. A non-zero value uniquely identifies the sub track locally within the track. A zero value (default) means that sub track ID is not assigned.
	AttributeList  []uint32 // 32 bits array, is a list, to the end of the box, of attributes. The attributes in this list should be used as descriptions of sub tracks or differentiating criteria for tracks and sub tracks in the same alternate or switch group
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/101
//
// Box Type: strd
// Container: Sub Track box (‘strk’)
// Mandatory: Yes
// Quantity: One
//
// This box contains objects that provide a definition of the sub track.
type SubTrackDefinition struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/81
//
// Box Type: meta
// Container: File, Movie Box (‘moov’), Track Box (‘trak’), or Additional Metadata Container Box (‘meco’)
// Mandatory: No
// Quantity: Zero or one (in File, ‘moov’, and ‘trak’), One or more (in ‘meco’)
type MetaBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	PrimaryResource PrimaryItemBox     // optional
	FileLocations   DataInformationBox // optional
	ItemLocations   ItemLocationBox    // optional
	Protections     ItemProtectionBox  // optional
	ItemInfos       ItemInfoBox        // optional
	IPMPControl     IPMPControlBox     // optional
	ItemRefs        ItemReferenceBox   // optional
	ItemData        ItemDataBox        // optional
	//OtherBoxes      []Box              // optional
}

type IPMPControlBox struct{}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/82
//
// Box Type: iloc
// Container: Meta box (‘meta’)
// Mandatory: No
// Quantity: Zero or one
type ItemLocationBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	OffsetSize     byte        // 4 bits,
	LengthSize     byte        // 4 bits,
	BaseOffsetSize byte        // 4 bits,
	IndexSize      byte        // 4 bits, if version == 1, index_size replace to reserved.
	ItemCount      uint16      // 16 bits,
	Table          interface{} // version == 1 -> ItemLocationTable1 , version == 2 -> ItemLocationTable2, ItemCount elements.
}

type ItemLocationTable1 struct {
	ItemID             uint16                     // 16 bits,
	Reserved           uint16                     // 12 bits,
	ConstructionMethod byte                       // 4 bits,
	DataReferenceIndex uint16                     // 16 bits,
	BaseOffset         interface{}                // unsigned int(base_offset_size*8),
	ExtentCount        uint16                     // 16 bits,
	ExtentTable        []ItemLocationExtentTable1 // Item Location Extent Table1, ExtentCount elements.
}

type ItemLocationTable2 struct {
	ItemID             uint16                     // 16 bits,
	DataReferenceIndex uint16                     // 16 bits,
	BaseOffset         interface{}                // unsigned int(base_offset_size*8),
	ExtentCount        uint16                     // 16 bits,
	ExtentTable        []ItemLocationExtentTable2 // Item Location Extent Table2, ExtentCount elements.
}

type ItemLocationExtentTable1 struct {
	ExtentIndex interface{} // unsigned int(index_size*8)
	ItemLocationExtentTable2
}

type ItemLocationExtentTable2 struct {
	ExtentOffset interface{} // unsigned int(offset_size*8)
	ExtentLength interface{} // unsigned int(length_size*8)
}

// for (i=0; i<item_count; i++) {
// 	unsigned int(16) item_ID;

// 	if (version == 1) {
// 		unsigned int(12) reserved = 0;
// 		unsigned int(4) construction_method;
// 	}

// 	unsigned int(16) data_reference_index;
// 	unsigned int(base_offset_size*8) base_offset;
// 	unsigned int(16) extent_count;

// 	for (j=0; j<extent_count; j++) {
// 		if ((version == 1) && (index_size > 0)) {
// 			unsigned int(index_size*8) extent_index;
// 		}

// 	unsigned int(offset_size*8) extent_offset;
// 	unsigned int(length_size*8) extent_length;
// }

// offset_size 			: is taken from the set {0, 4, 8} and indicates the length in bytes of the offset field.
// length_size 			: is taken from the set {0, 4, 8} and indicates the length in bytes of the length field.
// base_offset_size		: is taken from the set {0, 4, 8} and indicates the length in bytes of the base_offset field.
// index_size 			: is taken from the set {0, 4, 8} and indicates the length in bytes of the extent_index field.
// item_count 			: counts the number of resources in the following array.
// item_ID 				: is an arbitrary integer ‘name’ for this resource which can be used to refer to it (e.g. in a URL).
// construction_method	: is taken from the set 0 (file), 1 (idat) or 2 (item)
// data-reference-index	: is either zero (‘this file’) or a 1-based index into the data references in the data information box.
// base_offset 			: provides a base value for offset calculations within the referenced data. If
// base_offset_size 	: is 0, base_offset takes the value 0, i.e. it is unused.
// extent_count 		: provides the count of the number of extents into which the resource is fragmented; it must have the value 1 or greater
// extent_index 		: provides an index as defined for the construction method
// extent_offset 		: provides the absolute offset in bytes from the beginning of the containing file, of this item. If offset_size is 0, offset takes the value 0
// extent_length 		: provides the absolute length in bytes of this metadata item. If length_size is 0, length takes the value 0. If the value is 0, then length of the item is the length of the entire referenced file.

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/85
//
// Box Type: ipro
// Container: Meta box (‘meta’)
// Mandatory: No
// Quantity: Zero or one
//
// The item protection box provides an array of item protection information, for use by the Item Information Box.
type ItemProtectionBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	ProtectionCount uint16                // 16 bits,
	Table           []ItemProtectionTable // Item Protection Table, ProtectionCount elements.
}

type ItemProtectionTable struct {
	ProtectionInformation ProtectionSchemeInfoBox
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/92
//
// Box Types: sinf
// Container: Protected Sample Entry, or Item Protection Box (‘ipro’)
// Mandatory: Yes
// Quantity: One or More
type ProtectionSchemeInfoBox struct {
	MP4BoxHeader // standard header

	OriginalFormat OriginalFormatBox    //
	Type           SchemeTypeBox        // optional
	Info           SchemeInformationBox // optional
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/93
//
// Box Types: frma
// Container: Protection Scheme Information Box (‘sinf’) or Restricted Scheme Information Box (‘rinf’)
// Mandatory: Yes when used in a protected sample entry or in a restricted sample entry
// Quantity: Exactly one
//
// The Original Format Box ‘frma’ contains the four-character-code of the original un-transformed sample description:

type OriginalFormatBox struct {
	MP4BoxHeader // standard header

	DataFormat uint32 // 32 bits, is the four-character-code of the original un-transformed sample entry (e.g. “mp4v” if the stream contains protected or restricted MPEG-4 visual material).
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/93
//
// Box Types: schm
// Container: Protection Scheme Information Box (‘sinf’), Restricted Scheme Information Box (‘rinf’),
// or SRTP Process box (‘srpp‘)
// Mandatory: No
//
// Quantity: Zero or one in ‘sinf’, depending on the protection structure; Exactly one in ‘rinf’ and ‘srpp’
// The Scheme Type Box (‘schm’) identifies the protection or restriction scheme.
type SchemeTypeBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SchemeType    uint32 // 32 bits, is the code defining the protection or restriction scheme.
	SchemeVersion uint32 // 32 bits, is the version of the scheme (used to create the content)
}

// if (flags & 0x000001) {
// 	unsigned int(8) scheme_uri[]; // browser uri
// }

// scheme_URI : allows for the option of directing the user to a web-page if they do not have the scheme installed on their system. It is an absolute URI formed as a null-terminated string in UTF-8 characters.

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/94
//
// Box Types: schi
// Container: Protection Scheme Information Box (‘sinf’), Restricted Scheme Information Box (‘rinf’),
// or SRTP Process box (‘srpp‘)
// Mandatory: No
// Quantity: Zero or one
// The Scheme Information Box is a container Box that is only interpreted by the scheme being used. Any
// information the encryption or restriction system needs is stored here. The content of this box is a series of
// boxes whose type and format are defined by the scheme declared in the Scheme Type Box.
type SchemeInformationBox struct {
	MP4BoxHeader // standard header

	SchemeSpecificData []SchemeTypeBox
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/85
//
// Box Type: iinf
// Container: Meta Box (‘meta’)
// Mandatory: No
// Quantity: Zero or one

type ItemInfoBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint16          // 16 bits,
	ItemInfos  []ItemInfoEntry // EntryCount elements.
}

// Box Type: infe
type ItemInfoEntry struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	ItemID              uint16 // 16 bits
	ItemProtectionIndex uint16 // 16 bits
	ItemType            uint32 // 32 bits,
	ItemName            string // string,
	ContentType         string // string,
	ContentEncoding     string // string, optional
	ItemUriType         string // string,
	ExtensionType       uint32 // 32 bits, optional
	ItemInfoExtension          // optional
}

type ItemInfoExtension struct {
}

// if ((version == 0) || (version == 1)) {
// 	unsigned int(16) item_ID;
// 	unsigned int(16) item_protection_index
// 	string item_name;
// 	string content_type;
// 	string content_encoding; //optional
// }

// if (version == 1) {
// 	unsigned int(32) extension_type; //optional
// 	ItemInfoExtension(extension_type); //optional
// }

// if (version == 2) {
// 	unsigned int(16) item_ID;
// 	unsigned int(16) item_protection_index;
// 	unsigned int(32) item_type;
// 	string item_name;

// 	if (item_type==’mime’) {
// 		string content_type;
// 		string content_encoding; //optional
// 	} else if (item_type == ‘uri ‘) {
// 		string item_uri_type;
// 	}
// }

// item_id 					: contains either 0 for the primary resource (e.g., the XML contained in an ‘xml ‘ box) or the ID of the item for which the following information is defined.
// item_protection_index 	: contains either 0 for an unprotected item, or the one-based index into the item protection box defining the protection applied to this item (the first box in the item protection box has the index 1).
// item_name 				: is a null-terminated string in UTF-8 characters containing a symbolic name of the item (source file for file delivery transmissions).
// item_type 				: is a 32-bit value, typically 4 printable characters, that is a defined valid item type indicator, such as ‘mime’
// content_type 			: is a null-terminated string in UTF-8 characters with the MIME type of the item. If the item is content encoded (see below), then the content type refers to the item after content decoding.
// item_uri_type 			: is a string that is an absolute URI, that is used as a type indicator.
// content_encoding 		: is an optional null-terminated string in UTF-8 characters used to indicate that the binary file is encoded and needs to be decoded before interpreted. The values are as defined for Content-Encoding for HTTP/1.1. Some possible values are “gzip”, “compress” and “deflate”. An empty string indicates no content encoding. Note that the item is stored after the content encoding has been applied.
// extension_type 			: is a printable four-character code that identifies the extension fields of version 1 with respect to version 0 of the Item information entry.
// content_location 		: is a null-terminated string in UTF-8 characters containing the URI of the file as defined in HTTP/1.1 (RFC 2616).
// content_MD5 				: is a null-terminated string in UTF-8 characters containing an MD5 digest of the file. See HTTP/1.1 (RFC 2616) and RFC 1864.
// content_length 			: gives the total length (in bytes) of the (un-encoded) file.
// transfer_length 			: gives the total length (in bytes) of the (encoded) file. Note that transfer length is equal to content length if no content encoding is applied (see above).
// entry_count provides 	: a count of the number of entries in the following array.
// group_ID 				: indicates a file group to which the file item (source file) belongs. See 3GPP TS 26.346 for more details on file groups.

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/82
//
// Box Type: ‘xml ‘ or ‘bxml’
// Container: Meta box (‘meta’)
// Mandatory: No
// Quantity: Zero or one
//
// When the primary data is in XML format and it is desired that the XML be stored directly in the meta-box, one
// of these forms may be used. The Binary XML Box may only be used when there is a single well-defined
// binarization of the XML for that defined format as identified by the handler.
// Within an XML box the data is in UTF-8 format unless the data starts with a byte-order-mark (BOM), which
// indicates that the data is in UTF-16 format.
type XMLBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	XML string // string,
}

type BinaryXMLBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	Data []uint8 // 8 bits array,
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/93
//
// Box Type: pitm
// Container: Meta box (‘meta’)
// Mandatory: No
// Quantity: Zero or one
//
// For a given handler, the primary data may be one of the referenced items when it is desired that it be stored
// elsewhere, or divided into extents; or the primary metadata may be contained in the meta-box (e.g. in an XML
// box). Either this box must occur, or there must be a box within the meta-box (e.g. an XML box) containing the
// primary information in the format required by the identified handler.
type PrimaryItemBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	ItemID uint16 // 16 bits, is the identifier of the primary item
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/95
//
// Box Type: fiin
// Container: Meta Box (‘meta’)
// Mandatory: No
// Quantity: Zero or one
//
// The FD item information box is optional, although it is mandatory for files using FD hint tracks. It provides
// information on the partitioning of source files and how FD hint tracks are combined into FD sessions. Each
// partition entry provides details on a particular file partitioning, FEC encoding and associated File and FEC
// reservoirs. It is possible to provide multiple entries for one source file (identified by its item ID) if alternative
// FEC encoding schemes or partitionings are used in the file. All partition entries are implicitly numbered and
// the first entry has number 1.
type FDItemInformationBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint16            // 16 bits,
	PE         []PartitionEntry  // EntryCount elements.
	FDSGB      FDSessionGroupBox // optional
	GidToNameB GroupIdToNameBox  // optional
}

// Box Type: paen
type PartitionEntry struct {
	FPB   FilePartitionBox //
	FECRB FECReservoirBox  //optional
	FRB   FileReservoirBox //optional
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/99
//
// Box Type: fire
// Container: Partition Entry (‘paen’)
// Mandatory: No
// Quantity: Zero or One
//
// The File reservoir box associates the source file identified in the file partition box ('fpar') with File reservoirs
// stored as additional items. It contains a list that starts with the first File reservoir associated with the first
// source block of the source file and continues sequentially through the source blocks of the source file.
type FileReservoirBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint16               // 16 bits, gives the number of entries in the following list. An entry count here should match the total number or blocks in the corresponding file partition box.
	Table      []FileReservoirTable // EntryCount elements.
}

type FileReservoirTable struct {
	ItemID      uint16 // 16 bits, indicates the location of the File reservoir associated with a source block.
	SymbolCount uint32 // 32 bits, indicates the number of source symbols contained in the File reservoir.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/95
//
// Box Type: fpar
// Container: Partition Entry (‘paen’)
// Mandatory: Yes
// Quantity: Exactly one
//
// The File Partition box identifies the source file and provides a partitioning of that file into source blocks and
// symbols. Further information about the source file, e.g., filename, content location and group IDs, is contained
// in the Item Information box ('iinf'), where the Item Information entry corresponding to the item ID of the
// source file is of version 1 and includes a File Delivery Item Information Extension ('fdel').
type FilePartitionBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	ItemID                     uint16               // 16 bits,
	PacketPayloadSize          uint16               // 16 bits,
	Reserved                   uint8                // 8 bits,
	FECEncodingID              uint8                // 8 bits,
	FECInstanceID              uint16               // 16 bits,
	MaxSourceBlockLength       uint16               // 16 bits,
	EncodingSymbolLength       uint16               // 16 bits,
	MaxNumberOfEncodingSymbols uint16               // 16 bits,
	SchemeSpecificInfo         string               // string,
	EntryCount                 uint16               // 16 bits,
	Tanble                     []FilePartitionTable //File Partition Table, EntryCount elements.
}

type FilePartitionTable struct {
	BlockCount uint16 // 16 bits,
	BlockSize  uint32 // 32 bits,
}

// item_ID 							: references the item in the item location box ('iloc') that the file partitioning applies to.
// packet_payload_size 				: gives the target ALC/LCT or FLUTE packet payload size of the partitioning algorithm. Note that UDP packet payloads are larger, as they also contain ALC/LCT or FLUTE headers.
// FEC_encoding_ID 					: identifies the FEC encoding scheme and is subject to IANA registration (see RFC 5052). Note that i) value zero corresponds to the "Compact No-Code FEC scheme" also known as "Null-FEC" (RFC 3695); ii) value one corresponds to the “MBMS FEC” (3GPP TS 26.346); iii) for values in the range of 0 to 127, inclusive, the FEC scheme is Fully-Specified, whereas for values in the range of 128 to 255, inclusive, the FEC scheme is Under-Specified.
// FEC_instance_ID			 		: provides a more specific identification of the FEC encoder being used for an UnderSpecified FEC scheme. This value should be set to zero for Fully-Specified FEC schemes and shall be ignored when parsing a file with FEC_encoding_ID in the range of 0 to 127, inclusive. FEC_instance_ID is scoped by the FEC_encoding_ID. See RFC 5052 for further details.
// max_source_block_length 			: gives the maximum number of source symbols per source block.
// encoding_symbol_length 			: gives the size (in bytes) of one encoding symbol. All encoding symbols of one item have the same length, except the last symbol which may be shorter.
// max_number_of_encoding_symbols 	: gives the maximum number of encoding symbols that can be generated for a source block for those FEC schemes in which the maximum number of encoding symbols is relevant, such as FEC encoding ID 129 defined in RFC 5052. For those FEC schemes in which the maximum number of encoding symbols is not relevant, the semantics of this field is unspecified.
// scheme_specific_info				: is a base64-encoded null-terminated string of the scheme-specific object transfer information (FEC-OTI-Scheme-Specific-Info). The definition of the information depends on the FEC encoding ID.
// entry_count 						: gives the number of entries in the list of (block_count, block_size) pairs that provides a partitioning of the source file. Starting from the beginning of the file, each entry indicates how the next segment of the file is divided into source blocks and source symbols.
// block_count 						: indicates the number of consecutive source blocks of size block_size.
// block_size  						: indicates the size of a block (in bytes). A block_size that is not a multiple of the encoding_symbol_length symbol size indicates with Compact No-Code FEC that the last source symbols includes padding that is not stored in the item. With MBMS FEC (3GPP TS 26.346) the padding may extend across multiple symbols but the size of padding should never be more than encoding_symbol_length.

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/97
//
// Box Type: fecr
// Container: Partition Entry (‘paen’)
// Mandatory: No
// Quantity: Zero or One
//
// The FEC reservoir box associates the source file identified in the file partition box ('fpar') with FEC
// reservoirs stored as additional items. It contains a list that starts with the first FEC reservoir associated with
// the first source block of the source file and continues sequentially through the source blocks of the source file.
type FECReservoirBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint16              // 16 bits,
	Table      []FECReservoirTable // FEC Reservoir Table, EntryCount elements.
}

type FECReservoirTable struct {
	ItemID      uint16 // 16 bits, indicates the location of the FEC reservoir associated with a source block.
	SymbolCount uint32 // 32 bits, indicates the number of repair symbols contained in the FEC reservoir.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/97
//
// Box Type: segr
// Container: FD Information Box (‘fiin’)
// Mandatory: No
// Quantity: Zero or One
type FDSessionGroupBox struct {
	MP4BoxHeader // standard header

	NumSessionGroups uint16                // 16 bits,
	Table            []FDSessionGroupTable // FD Session Group Table, NumSessionGroups elements.
}

type FDSessionGroupTable struct {
	EntryCount                uint8                       // 8 bits,
	GIDTable                  []FDSessionGroupIDTable     // FDSession Group ID Table, EntryCount elements.
	NumChannelsInSessionGroup uint16                      // 16 bits
	HTIDTable                 []FDSessionHintTrackIDTable // FDSession Hint Track ID Table, NumChannelsInSessionGroup elements.
}

type FDSessionGroupIDTable struct {
	GroupID uint32 // 32 bits
}

type FDSessionHintTrackIDTable struct {
	HintTrackID uint32 // 32 bits
}

// for(i=0; i < num_session_groups; i++) {
// 	unsigned int(8) entry_count;

// 	for (j=0; j < entry_count; j++) {
// 		unsigned int(32) group_ID;
// 	}

// 	unsigned int(16) num_channels_in_session_group;

// 	for(k=0; k < num_channels_in_session_group; k++) {
// 		unsigned int(32) hint_track_id;
// 	}
// }

// num_session_groups 				: specifies the number of session groups.
// entry_count 						: gives the number of entries in the following list comprising all file groups that the session group complies with. The session group contains all files included in the listed file groups as specified by the item information entry of each source file. Note that the FDT for the session group should only contain those groups that are listed in this structure.
// group_ID 						: indicates a file group that the session group complies with.
// num_channels_in_session_groups 	: specifies the number of channels in the session group. The value of num_channels_in_session_groups shall be a positive integer.
// hint_track_ID 					: specifies the track ID of the FD hint track belonging to a particular session group. Note that one FD hint track corresponds to one LCT channel.

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/98
//
// Box Type: gitn
// Container: FD Information Box (‘fiin’)
// Mandatory: No
// Quantity: Zero or One
//
// The Group ID to Name box associates file group names to file group IDs used in the version 1 item
// information entries in the item information box ('iinf').
type GroupIdToNameBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	EntryCount uint16             // 16 bits, gives the number of entries in the following list.
	Table      []GroupIdToNameBox // Group Id To Name Table, EntryCount elements.
}

type GroupIdToNameTable struct {
	GroupID   uint32 // 32 bits, indicates a file group.
	GroupName string // string, is a null-terminated string in UTF-8 characters containing a file group name.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/90
//
// Box Type: idat
// Container: Metadata box (‘meta’)
// Mandatory: No
// Quantity: Zero or one
//
// This box contains the data of metadata items that use the construction method indicating that an item’s data
// extents are stored within this box.
type ItemDataBox struct {
	MP4BoxHeader // standard header

	Data []byte // 8 bits array, is the contained meta data
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/91
//
// Box Type: iref
// Container: Metadata box (‘meta’)
// Mandatory: No
// Quantity: Zero or one
//
// The item reference box allows the linking of one item to others via typed references. All the references for one
// item of a specific type are collected into a single item type reference box, whose type is the reference type,
// and which has a ‘from item ID’ field indicating which item is linked. The items linked to are then represented by
// an array of ‘to item ID’s. All these single item type reference boxes are then collected into the item reference
// box. The reference types defined for the track reference box defined in 8.3.3 may be used here if appropriate,
// or other registered reference types.
type ItemReferenceBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SITRB []SingleItemTypeReferenceBox
}

type SingleItemTypeReferenceBox struct {
	MP4BoxHeader // standard header

	FromItemID     uint16                         // 16 bits, contains the ID of the item that refers to other items
	ReferenceCount uint16                         // 16 bits, is the number of references
	Table          []SingleItemTypeReferenceTable // Single Item Type Reference Table, ReferenceCount elements.
}

type SingleItemTypeReferenceTable struct {
	ToItemID uint16 // 16 bits, contains the ID of the item referred to
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/87
//
// Box Type: meco
// Container: File, Movie Box (‘moov’), or Track Box (‘trak’)
// Mandatory: No
// Quantity: Zero or one
type AdditionalMetadataContainerBox struct {
	MP4BoxHeader // standard header
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/88
//
// Box Type: mere
// Container: Additional Metadata Container Box (‘meco’)
// Mandatory: No
// Quantity: Zero or more
//
// The metabox relation box indicates a relation between two meta boxes at the same level, i.e., the top level of
// the file, the Movie Box, or Track Box. The relation between two meta boxes is unspecified if there is no
// metabox relation box for those meta boxes. Meta boxes are referenced by specifying their handler types.
type MetaboxRelationBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	FirstMetaboxHandlerType  uint32 // 32 bits, indicates the first meta box to be related.
	SecondMetaboxHandlerType uint32 // 32 bits,  indicates the second meta box to be related.
	MetaboxRelation          uint8  // 8 bits, indicates the relation between the two meta boxes.
}

// metabox_relation indicates the relation between the two meta boxes. The following values are defined:
// 1 The relationship between the boxes is unknown (which is the default when this box is not present);
// 2 the two boxes are semantically un-related (e.g., one is presentation, the other annotation);
// 3 the two boxes are semantically related but complementary (e.g., two disjoint sets of meta-data expressed in two different meta-data systems);
// 4 the two boxes are semantically related but overlap (e.g., two sets of meta-data neither of which is a subset of the other); neither is ‘preferred’ to the other;
// 5 the two boxes are semantically related but the second is a proper subset or weaker version of the first; the first is preferred;
// 6 the two boxes are semantically related and equivalent (e.g., two essentially identical sets of meta-data expressed in two different meta-data systems).

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/105
//
// Box Type: styp
// Container: File
// Mandatory: No
// Quantity: Zero or more
//
// If segments are stored in separate files (e.g. on a standard HTTP server) it is recommended that these
// ‘segment files’ contain a segment-type box, which must be first if present, to enable identification of those files,
// and declaration of the specifications with which they are compliant.
// A segment type has the same format as an 'ftyp' box [4.3], except that it takes the box type 'styp'. The
// brands within it may include the same brands that were included in the 'ftyp' box that preceded the
// ‘moov’ box, and may also include additional brands to indicate the compatibility of this segment with various
// specification(s).
// Valid segment type boxes shall be the first box in a segment. Segment type boxes may be removed if
// segments are concatenated (e.g. to form a full file), but this is not required. Segment type boxes that are not
// first in their files may be ignored.

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/106
//
// Box Type: sidx
// Container: File
// Mandatory: No
// Quantity: Zero or more
type SegmentIndexBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	ReferenceID              uint32              // 32 bits,
	TimeScale                uint32              // 32 bits,
	EarliestPresentationTime interface{}         // uint32 or uint64,
	FirstOffset              interface{}         // uint32 or uint64,
	Reserved                 uint16              // 16 bits,
	ReferenceCount           uint16              // 16 bits,
	Table                    []SegmentIndexTable // Segment Index Table, ReferenceCount elements
}

type SegmentIndexTable struct {
	ReferenceType      byte   // 1 bit
	ReferencedSize     uint32 // 32 bits
	SubSegmentDuration uint32 // 32 bits,
	StartsWithSAP      byte   // 1 bit
	SAPType            byte   // 3 bits,
	SAPDeltaTime       uint32 // 28 bits,
}

// if (version==0) {
// 	unsigned int(32) earliest_presentation_time;
// 	unsigned int(32) first_offset;
// }
// else {
// 	unsigned int(64) earliest_presentation_time;
// 	unsigned int(64) first_offset;
// }

// unsigned int(16) reserved = 0;
// unsigned int(16) reference_count;

// for(i=1; i <= reference_count; i++)
// {
// 	bit (1) reference_type;
// 	unsigned int(31) referenced_size;
// 	unsigned int(32) subsegment_duration;
// 	bit(1) starts_with_SAP;
// 	unsigned int(3) SAP_type;
// 	unsigned int(28) SAP_delta_time;
// }

// reference_ID 				: provides the stream ID for the reference stream; if this Segment Index box is referenced from a “parent” Segment Index box, the value of reference_ID shall be the same as the value of reference_ID of the “parent” Segment Index box;
// timescale 					: provides the timescale, in ticks per second, for the time and duration fields within this box; it is recommended that this match the timescale of the reference stream or track; for files based on this specification, that is the timescale field of the Media Header Box of the track;
// earliest_presentation_time 	: is the earliest presentation time of any access unit in the reference stream in the first subsegment, in the timescale indicated in the timescale field;
// first_offset 				: is the distance in bytes, in the file containing media, from the anchor point, to the first byte of the indexed material;
// reference_count 				: provides the number of referenced items;
// reference_type 				: when set to 1 indicates that the reference is to a segment index (‘sidx’) box; otherwise the reference is to media content (e.g., in the case of files based on this specification, to a movie fragment box); if a separate index segment is used, then entries with reference type 1 are in the index segment, and entries with reference type 0 are in the media file;
// referenced_size 				: the distance in bytes from the first byte of the referenced item to the first byte of the next referenced item, or in the case of the last entry, the end of the referenced material;
// subsegment_duration			: when the reference is to Segment Index box, this field carries the sum of the subsegment_duration fields in that box; when the reference is to a subsegment, this field carries the difference between the earliest presentation time of any access unit of the reference stream in the next subsegment (or the first subsegment of the next segment, if this is the last subsegment of the segment, or the end presentation time of the reference stream if this is the last subsegment of the stream) and the earliest presentation time of any access unit of the reference stream in the referenced subsegment; the duration is in the same units as earliest_presentation_time;
// starts_with_SAP 				: indicates whether the referenced subsegments start with a SAP. For the detailed semantics of this field in combination with other fields, see the table below.
// SAP_type 					: indicates a SAP type as specified in Annex I, or the value 0. Other type values are reserved. For the detailed semantics of this field in combination with other fields, see the table below.
// SAP_delta_time 				: indicates TSAP of the first SAP, in decoding order, in the referenced subsegment for the reference stream. If the referenced subsegments do not contain a SAP, SAP_delta_time is reserved with the value 0; otherwise SAP_delta_time is the difference between the earliest presentation time of the subsegment, and the TSAP (note that this difference may be zero, in the case that the subsegment starts with a SAP).

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/109
//
// Box Type: ssix
// Container: File
// Mandatory: No
// Quantity: Zero or more
type SubsegmentIndexBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	SubSegmentCount uint32                 // 32 bits, is a positive integer specifying the number of subsegments for which partial subsegment information is specified in this box. subsegment_count shall be equal to reference_count (i.e., the number of movie fragment references) in the immediately preceding Segment Index box.
	Table           []SubsegmentIndexTable // Subsegment Index Table, SubSegmentCount elements.
}

type SubsegmentIndexTable struct {
	RangesCount uint32                  // 32 bits, specifies the number of partial subsegment levels into which the media data is grouped. This value shall be greater than or equal to 2.
	Rtable      []SubsegmentRangesTable // Subsegment Ranges Table, RangesCount elements.
}

type SubsegmentRangesTable struct {
	level      uint8   // 8 bits, specifies the level to which this partial subsegment is assigned.
	range_size [3]byte // 24 bits, indicates the size of the partial subsegment.
}

// -------------------------------------------------------------------------------------------------------

//
// ISO_IEC_14496-12_2012.pdf Page/111
//
// Box Type: prft
// Container: File
// Mandatory: No
// Quantity: Zero or more
type ProducerReferenceTimeBox struct {
	MP4BoxHeader     // standard header
	MP4FullBoxHeader // full box header

	ReferenceTrackID uint32      // 32 bits, provides the track_ID for the reference track.
	NtpTimestamp     uint64      // 64 bits, indicates a UTC time in NTP format corresponding to decoding_time.
	MediaTime        interface{} // uint32 or uint64, corresponds to the same time as ntp_timestamp, but in the time units used for the reference track, and is measured on this media clock as the media is produced.
}

// if (version==0) {
// 	unsigned int(32) media_time;
// } else {
// 	unsigned int(64) media_time;
// }

// -------------------------------------------------------------------------------------------------------
