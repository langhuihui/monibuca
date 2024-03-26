package pkg

type NetStream struct {
	*NetConnection
	StreamID uint32
}

func (ns *NetStream) Begin() {
	ns.SendStreamID(RTMP_USER_STREAM_BEGIN, ns.StreamID)
}
