package box

type Sample struct {
	KeyFrame bool
	Data     []byte
	DTS, PTS uint64
	Offset   int64
	Size     int
}
