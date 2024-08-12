package pkg

import "errors"

var (
	ErrNotFound                 = errors.New("not found")
	ErrStopFromAPI              = errors.New("stop from api")
	ErrStreamExist              = errors.New("stream exist")
	ErrKick                     = errors.New("kick")
	ErrDiscard                  = errors.New("discard")
	ErrPublishTimeout           = errors.New("publish timeout")
	ErrPublishIdleTimeout       = errors.New("publish idle timeout")
	ErrPublishDelayCloseTimeout = errors.New("publish delay close timeout")
	ErrPushRemoteURLExist       = errors.New("push remote url exist")
	ErrSubscribeTimeout         = errors.New("subscribe timeout")
	ErrRestart                  = errors.New("restart")
	ErrInterrupt                = errors.New("interrupt")
	ErrUnsupportCodec           = errors.New("unsupport codec")
	ErrMuted                    = errors.New("muted")
	ErrLost                     = errors.New("lost")
	ErrRetryRunOut              = errors.New("retry run out")
	ErrRecordSamePath           = errors.New("record same path")
)