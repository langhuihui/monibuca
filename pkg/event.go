package pkg

type Event[T any] struct {
	Type string
	Data T
}

