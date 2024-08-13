package webrtc

import (
	. "github.com/pion/webrtc/v3"
	"m7s.live/m7s/v5/pkg/util"
)

type Connection struct {
	util.MarcoTask
	*PeerConnection
	SDP string
	// LocalSDP *sdp.SessionDescription
}

func (IO *Connection) GetAnswer() (string, error) {
	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := IO.CreateAnswer(nil)
	if err != nil {
		return "", err
	}
	// IO.LocalSDP, err = answer.Unmarshal()
	// if err != nil {
	// 	return "", err
	// }
	gatherComplete := GatheringCompletePromise(IO.PeerConnection)
	if err := IO.SetLocalDescription(answer); err != nil {
		return "", err
	}
	<-gatherComplete
	return IO.LocalDescription().SDP, nil
}

func (IO *Connection) Dispose() {
	if IO.PeerConnection == nil {
		IO.PeerConnection.Close()
	}
}
