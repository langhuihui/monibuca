package rtmp

import (
	"bufio"
	"fmt"
	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/avformat"
	"log"
	"net"
	"strings"
	"time"
)

type RTMP struct {
	InputStream
}

func ListenRtmp(addr string) error {
	defer log.Println("rtmp server start!")
	// defer fmt.Println("server start!")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	var tempDelay time.Duration
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				fmt.Printf("rtmp: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return err
		}

		tempDelay = 0
		go processRtmp(conn)
	}
	return nil
}

var gstreamid = uint32(64)

func processRtmp(conn net.Conn) {
	var room *Room
	streams := make(map[uint32]*OutputStream)
	defer func() {
		conn.Close()
		if room != nil {
			room.Cancel()
		}
	}()
	var totalDuration uint32
	nc := &NetConnection{
		ReadWriter:         bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
		writeChunkSize:     RTMP_DEFAULT_CHUNK_SIZE,
		readChunkSize:      RTMP_DEFAULT_CHUNK_SIZE,
		rtmpHeader:         make(map[uint32]*ChunkHeader),
		incompleteRtmpBody: make(map[uint32][]byte),
		bandwidth:          RTMP_MAX_CHUNK_SIZE << 3,
		nextStreamID: func(u uint32) uint32 {
			gstreamid++
			return gstreamid
		},
	}
	/* Handshake */
	if MayBeError(Handshake(nc.ReadWriter)) {
		return
	}
	if MayBeError(nc.OnConnect()) {
		return
	}
	for {
		if msg, err := nc.RecvMessage(); err == nil {
			if msg.MessageLength <= 0 {
				continue
			}
			switch msg.MessageTypeID {
			case RTMP_MSG_AMF0_COMMAND:
				if msg.MsgData == nil {
					break
				}
				cmd := msg.MsgData.(Commander).GetCommand()
				switch cmd.CommandName {
				case "createStream":
					nc.streamID = nc.nextStreamID(msg.ChunkStreamID)
					err = nc.SendMessage(SEND_CREATE_STREAM_RESPONSE_MESSAGE, cmd.TransactionId)
					if MayBeError(err) {
						return
					}
				case "publish":
					pm := msg.MsgData.(*PublishMessage)
					streamPath := nc.appName + "/" + strings.Split(pm.PublishingName, "?")[0]
					pub := new(RTMP)
					if pub.Publish(streamPath, pub) {
						pub.FirstScreen = make([]*avformat.AVPacket, 0)
						room = pub.Room
						err = nc.SendMessage(SEND_STREAM_BEGIN_MESSAGE, nil)
						err = nc.SendMessage(SEND_PUBLISH_START_MESSAGE, newPublishResponseMessageData(nc.streamID, NetStream_Publish_Start, Level_Status))
					} else {
						err = nc.SendMessage(SEND_PUBLISH_RESPONSE_MESSAGE, newPublishResponseMessageData(nc.streamID, Level_Error, NetStream_Publish_BadName))
					}
				case "play":
					pm := msg.MsgData.(*PlayMessage)
					streamPath := nc.appName + "/" + strings.Split(pm.StreamName, "?")[0]
					nc.writeChunkSize = 512
					stream := &OutputStream{SendHandler: func(packet *avformat.SendPacket) (err error) {
						switch true {
						case packet.Packet.IsADTS:
							tagPacket := avformat.NewAVPacket(RTMP_MSG_AUDIO)
							tagPacket.Payload = avformat.ADTSToAudioSpecificConfig(packet.Packet.Payload)
							err = nc.SendMessage(SEND_FULL_AUDIO_MESSAGE, tagPacket)
							ADTSLength := 7 + (int(packet.Packet.Payload[1]&1) << 1)
							if len(packet.Packet.Payload) > ADTSLength {
								contentPacket := avformat.NewAVPacket(RTMP_MSG_AUDIO)
								contentPacket.Timestamp = packet.Timestamp
								contentPacket.Payload = make([]byte, len(packet.Packet.Payload)-ADTSLength+2)
								contentPacket.Payload[0] = 0xAF
								contentPacket.Payload[1] = 0x01 //raw AAC
								copy(contentPacket.Payload[2:], packet.Packet.Payload[ADTSLength:])
								err = nc.SendMessage(SEND_AUDIO_MESSAGE, contentPacket)
							}
						case packet.Packet.IsAVCSequence:
							err = nc.SendMessage(SEND_FULL_VDIEO_MESSAGE, packet)
						case packet.Packet.Type == RTMP_MSG_VIDEO:
							err = nc.SendMessage(SEND_VIDEO_MESSAGE, packet)
						case packet.Packet.IsAACSequence:
							err = nc.SendMessage(SEND_FULL_AUDIO_MESSAGE, packet)
						case packet.Packet.Type == RTMP_MSG_AUDIO:
							err = nc.SendMessage(SEND_AUDIO_MESSAGE, packet)
						}
						return nil
					}}
					stream.Type = "RTMP"
					stream.ID = fmt.Sprintf("%s|%d", conn.RemoteAddr().String(), nc.streamID)
					err = nc.SendMessage(SEND_CHUNK_SIZE_MESSAGE, uint32(nc.writeChunkSize))
					err = nc.SendMessage(SEND_STREAM_IS_RECORDED_MESSAGE, nil)
					err = nc.SendMessage(SEND_STREAM_BEGIN_MESSAGE, nil)
					err = nc.SendMessage(SEND_PLAY_RESPONSE_MESSAGE, newPlayResponseMessageData(nc.streamID, NetStream_Play_Reset, Level_Status))
					err = nc.SendMessage(SEND_PLAY_RESPONSE_MESSAGE, newPlayResponseMessageData(nc.streamID, NetStream_Play_Start, Level_Status))
					if err == nil {
						streams[nc.streamID] = stream
						go stream.Play(streamPath)
					} else {
						return
					}
				case "closeStream":
					cm := msg.MsgData.(*CURDStreamMessage)
					if stream, ok := streams[cm.StreamId]; ok {
						stream.Cancel()
						delete(streams, cm.StreamId)
					}
				}
			case RTMP_MSG_AUDIO:
				pkt := avformat.NewAVPacket(RTMP_MSG_AUDIO)
				if msg.Timestamp == 0xffffff {
					totalDuration += msg.ExtendTimestamp
				} else {
					totalDuration += msg.Timestamp // 绝对时间戳
				}
				pkt.Timestamp = totalDuration
				pkt.Payload = msg.Body
				room.PushAudio(pkt)
			case RTMP_MSG_VIDEO:
				pkt := avformat.NewAVPacket(RTMP_MSG_VIDEO)
				if msg.Timestamp == 0xffffff {
					totalDuration += msg.ExtendTimestamp
				} else {
					totalDuration += msg.Timestamp // 绝对时间戳
				}
				pkt.Timestamp = totalDuration
				pkt.Payload = msg.Body
				room.PushVideo(pkt)
			}
			msg.Recycle()
		} else {
			return
		}
	}
}
