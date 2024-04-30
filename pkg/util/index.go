package util

func Conditoinal[T any](cond bool, t, f T) T {
	if cond {
		return t
	} else {
		return f
	}
}

// Bit1 检查字节中的某一位是否为1 |0|1|2|3|4|5|6|7|
func Bit1(b byte, index int) bool {
	return b&(1<<(7-index)) != 0
}

func LenOfBuffers(b [][]byte) (n int) {
	for _, bb := range b {
		n += len(bb)
	}
	return
}
