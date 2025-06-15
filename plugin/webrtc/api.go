package plugin_webrtc

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	. "github.com/pion/webrtc/v4"
	. "m7s.live/v5/plugin/webrtc/pkg"
)

// https://datatracker.ietf.org/doc/html/draft-ietf-wish-whip
func (conf *WebRTCPlugin) servePush(w http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	rawQuery := r.URL.RawQuery
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		auth = auth[len("Bearer "):]
		if rawQuery != "" {
			rawQuery += "&bearer=" + auth
		} else {
			rawQuery = "bearer=" + auth
		}
		conf.Info("push", "stream", streamPath, "bearer", auth)
	}
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/webrtc/api/stop/push/"+streamPath)
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	if rawQuery != "" {
		streamPath += "?" + rawQuery
	}
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conn := MultipleConnection{
		PLI: conf.PLI,
	}
	conn.SDP = string(bytes)
	conn.Logger = conf.Logger

	if conn.PeerConnection, err = conf.CreatePC(SessionDescription{
		Type: SDPTypeOffer,
		SDP:  conn.SDP,
	}, Configuration{
		ICEServers: conf.ICEServers,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if conn.Publisher, err = conf.Publish(conf.Context, streamPath); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conn.Publisher.RemoteAddr = r.RemoteAddr
	conf.AddTask(&conn)
	if err = conn.WaitStarted(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if answer, err := conn.GetAnswer(); err == nil {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, answer.SDP)
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func (conf *WebRTCPlugin) servePlay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/sdp")
	streamPath := r.PathValue("streamPath")
	rawQuery := r.URL.RawQuery
	var conn MultipleConnection
	conn.EnableDC = conf.EnableDC
	bytes, err := io.ReadAll(r.Body)
	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}()
	if err != nil {
		return
	}
	conn.SDP = string(bytes)
	// Check if client supports H265
	if strings.Contains(strings.ToLower(conn.SDP), "h265") {
		conn.SupportsH265 = true
	}

	conn.PeerConnection, err = conf.CreatePC(SessionDescription{
		Type: SDPTypeOffer,
		SDP:  conn.SDP,
	}, Configuration{
		ICEServers: conf.ICEServers,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rawQuery != "" {
		streamPath += "?" + rawQuery
	}
	if conn.Subscriber, err = conf.Subscribe(conf.Context, streamPath); err != nil {
		return
	}
	conn.Subscriber.RemoteAddr = r.RemoteAddr
	conf.AddTask(&conn)
	if err = conn.WaitStarted(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if sdp, err := conn.GetAnswer(); err == nil {
		w.Write([]byte(sdp.SDP))
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
