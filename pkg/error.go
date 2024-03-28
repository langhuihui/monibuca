package pkg

import "errors"

var (
	ErrStreamExist              = errors.New("stream exist")
	ErrKick                     = errors.New("kick")
	ErrDiscard                  = errors.New("discard")
	ErrPublishTimeout           = errors.New("publish timeout")
	ErrPublishIdleTimeout       = errors.New("publish idle timeout")
	ErrPublishDelayCloseTimeout = errors.New("publish delay close timeout")
	ErrSubscribeTimeout         = errors.New("subscribe timeout")
)
