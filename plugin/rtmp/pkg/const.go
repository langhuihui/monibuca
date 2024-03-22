package pkg

const (
	PacketTypeSequenceStart = iota
	PacketTypeCodedFrames
	PacketTypeSequenceEnd
	PacketTypeCodedFramesX
	PacketTypeMetadata
	PacketTypeMPEG2TSSequenceStart
)