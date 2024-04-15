package rtmp

import (
	"net/url"
	"strings"

	"m7s.live/m7s/v5"
)

type RTMPPuller struct {
	NetStream
}

func (puller *RTMPPuller) Connect(p *m7s.Puller) (err error) {
	if puller.NetConnection, err = NewRTMPClient(p.RemoteURL, p.Publisher.Logger, 4096); err == nil {
		puller.Info("connect", "remoteURL", p.RemoteURL)
	}
	return
}

func (puller *RTMPPuller) Pull(p *m7s.Puller) (err error) {
	defer puller.Close()
	err = puller.SendMessage(RTMP_MSG_AMF0_COMMAND, &CommandMessage{"createStream", 2})
	for err == nil {
		msg, err := puller.RecvMessage()
		if err != nil {
			return err
		}
		switch msg.MessageTypeID {
		case RTMP_MSG_AUDIO:
			p.WriteAudio(&RTMPAudio{msg.AVData})
			msg.AVData = RTMPData{}
			msg.AVData.ScalableMemoryAllocator = puller.NetConnection.ReadPool
		case RTMP_MSG_VIDEO:
			p.WriteVideo(&RTMPVideo{msg.AVData})
			msg.AVData = RTMPData{}
			msg.AVData.ScalableMemoryAllocator = puller.NetConnection.ReadPool
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
					URL, _ := url.Parse(p.RemoteURL)
					ps := strings.Split(URL.Path, "/")
					p.Args = URL.Query()
					m.StreamName = ps[len(ps)-1]
					if len(p.Args) > 0 {
						m.StreamName += "?" + p.Args.Encode()
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
