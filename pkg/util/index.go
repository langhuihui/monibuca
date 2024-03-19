package util

func Conditoinal[T any](cond bool, t, f T) T {
	if cond {
		return t
	} else {
		return f
	}
}
