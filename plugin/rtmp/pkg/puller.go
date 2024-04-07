package rtmp

import (
	"net/url"
	"strings"

	"m7s.live/m7s/v5"
)

type RTMPPuller struct {
	*m7s.Puller
	NetStream
}

func (puller *RTMPPuller) Connect() (err error) {
	if puller.NetConnection, err = NewRTMPClient(puller.RemoteURL, puller.Publisher.Logger, 4096); err == nil {
		puller.Closer = puller.NetConnection.Conn
		puller.Info("connect", "remoteURL", puller.RemoteURL)
	}
	return
}

func (puller *RTMPPuller) Disconnect() {
	if puller.NetConnection != nil {
		puller.NetConnection.Close()
	}
}

func (puller *RTMPPuller) Pull() (err error) {
	err = puller.SendMessage(RTMP_MSG_AMF0_COMMAND, &CommandMessage{"createStream", 2})
	for err == nil {
		msg, err := puller.RecvMessage()
		if err != nil {
			return err
		}
		switch msg.MessageTypeID {
		case RTMP_MSG_AUDIO:
			puller.WriteAudio(&RTMPAudio{msg.AVData})
			msg.AVData = RTMPData{}
			msg.AVData.ScalableMemoryAllocator = puller.NetConnection.ByteChunkPool
		case RTMP_MSG_VIDEO:
			puller.WriteVideo(&RTMPVideo{msg.AVData})
			msg.AVData = RTMPData{}
			msg.AVData.ScalableMemoryAllocator = puller.NetConnection.ByteChunkPool
		case RTMP_MSG_AMF0_COMMAND:
			cmd := msg.MsgData.(Commander).GetCommand()
			switch cmd.CommandName {
			case "_result":
				if response, ok := msg.MsgData.(*ResponseCreateStreamMessage); ok {
					puller.StreamID = response.StreamId
					m := &PlayMessage{}
					m.StreamId = response.StreamId
					m.TransactionId = 4
					m.CommandMessage.CommandName = "play"
					URL, _ := url.Parse(puller.RemoteURL)
					ps := strings.Split(URL.Path, "/")
					puller.Args = URL.Query()
					m.StreamName = ps[len(ps)-1]
					if len(puller.Args) > 0 {
						m.StreamName += "?" + puller.Args.Encode()
					}
					puller.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
					// if response, ok := msg.MsgData.(*ResponsePlayMessage); ok {
					// 	if response.Object["code"] == "NetStream.Play.Start" {

					// 	} else if response.Object["level"] == Level_Error {
					// 		return errors.New(response.Object["code"].(string))
					// 	}
					// } else {
					// 	return errors.New("pull faild")
					// }
				}
			}
		}
	}
	return
}
