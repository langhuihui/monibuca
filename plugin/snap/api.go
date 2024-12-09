package plugin_snap

import (
	"bytes"
	"net/http"
	"strconv"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	snap "m7s.live/v5/plugin/snap/pkg"
)

func (t *SnapPlugin) doSnap(rw http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	targetStreamPath := streamPath

	if !t.Server.Streams.Has(streamPath) {
		http.Error(rw, pkg.ErrNotFound.Error(), http.StatusNotFound)
		return
	}

	buf := new(bytes.Buffer)
	transformer := snap.NewTransform().(*snap.Transformer)
	transformer.TransformJob.Init(transformer, &t.Plugin, streamPath, config.Transform{
		Output: []config.TransfromOutput{
			{
				Target:     targetStreamPath,
				StreamPath: targetStreamPath,
				Conf:       buf,
			},
		},
	}).WaitStarted()

	transformer.TriggerSnap()
	if err := transformer.Run(); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "image/jpeg")
	rw.Header().Set("Content-Length", strconv.Itoa(buf.Len()))

	if _, err := buf.WriteTo(rw); err != nil {
		t.Error("write response error", err)
		return
	}
}

func (config *SnapPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/{streamPath...}": config.doSnap,
	}
}
