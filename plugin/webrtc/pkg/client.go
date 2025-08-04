package webrtc

import (
	"errors"
	"io"
	"net/http"
	"strings"

	. "github.com/pion/webrtc/v4"
	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
)

const (
	DIRECTION_PULL = "pull"
	DIRECTION_PUSH = "push"
)

type PullRequest struct {
	Tracks []TrackInfo `json:"tracks"`
}

type Client struct {
	MultipleConnection
}

func (c *Client) Start() (err error) {
	var api *API
	api, err = CreateAPI(nil, SettingEngine{})
	if err != nil {
		return errors.Join(err, errors.New("create api failed"))
	}
	c.PeerConnection, err = api.NewPeerConnection(Configuration{
		ICEServers:         ICEServers,
		BundlePolicy:       BundlePolicyMaxBundle,
		ICETransportPolicy: ICETransportPolicyAll,
	})
	return c.MultipleConnection.Start()
}

// WHIPClient is a client that pushes media to the server
type WHIPClient struct {
	Client
	pushCtx m7s.PushJob
}

func (c *WHIPClient) GetPushJob() *m7s.PushJob {
	return &c.pushCtx
}

// WHEPClient is a client that pulls media from the server
type WHEPClient struct {
	Client
	pullCtx m7s.PullJob
}

func (c *WHEPClient) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func (c *WHEPClient) Start() (err error) {
	err = c.pullCtx.Publish()
	if err != nil {
		return
	}
	c.Publisher = c.pullCtx.Publisher
	c.pullCtx.GoToStepConst(StepWebRTCInit)
	err = c.Client.Start()
	if err != nil {
		return
	}
	// u, _ := url.Parse(c.pullCtx.RemoteURL)
	// c.ApiBase, _, _ = strings.Cut(c.pullCtx.RemoteURL, "?")
	c.Receive()
	if c.pullCtx.PublishConfig.PubVideo {
		var transeiver *RTPTransceiver
		transeiver, err = c.AddTransceiverFromKind(RTPCodecTypeVideo, RTPTransceiverInit{
			Direction: RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			return
		}
		c.Info("webrtc add video transceiver", "transceiver", transeiver.Mid())
	}

	if c.pullCtx.PublishConfig.PubAudio {
		var transeiver *RTPTransceiver
		transeiver, err = c.AddTransceiverFromKind(RTPCodecTypeAudio, RTPTransceiverInit{
			Direction: RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			return
		}
		c.Info("webrtc add audio transceiver", "transceiver", transeiver.Mid())
	}

	c.pullCtx.GoToStepConst(StepOfferCreate)
	var sdpBody SDPBody
	sdpBody.SessionDescription, err = c.GetOffer()
	if err != nil {
		return
	}

	c.pullCtx.GoToStepConst(StepSessionCreate)
	var res *http.Response
	res, err = http.DefaultClient.Post(c.pullCtx.RemoteURL, "application/sdp", strings.NewReader(sdpBody.SessionDescription.SDP))
	if err != nil {
		return
	}
	c.pullCtx.GoToStepConst(StepNegotiation)
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		err = errors.New(res.Status)
		return
	}
	var sd SessionDescription
	sd.Type = SDPTypeAnswer
	var body util.Buffer
	io.Copy(&body, res.Body)
	sd.SDP = string(body)
	if err = c.SetRemoteDescription(sd); err != nil {
		return
	}
	c.pullCtx.GoToStepConst(StepNegotiation)
	return
}

func NewPuller(conf config.Pull) m7s.IPuller {
	if strings.HasPrefix(conf.URL, "https://rtc.live.cloudflare.com") {
		return NewCFClient(DIRECTION_PULL)
	}
	client := &WHEPClient{}
	client.pullCtx.SetProgressStepsDefs(webrtcPullSteps)
	return client
}

func NewPusher() m7s.IPusher {
	return &WHIPClient{}
}
