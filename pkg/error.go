package pkg

import "errors"

var (
	ErrStreamExist = errors.New("stream exist")
	ErrKick        = errors.New("kick")
	ErrDiscard     = errors.New("discard")
)
