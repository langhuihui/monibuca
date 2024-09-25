package plugin_sei

import (
	"io"
	"net/http"
	"strconv"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
	sei "m7s.live/m7s/v5/plugin/sei/pkg"
)

var _ = m7s.InstallPlugin[SEIPlugin](sei.NewTransform)

type SEIPlugin struct {
	m7s.Plugin
}

func (conf *SEIPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/insert/{streamPath...}": conf.api_insert,
	}
}

func (conf *SEIPlugin) api_insert(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	streamPath := r.PathValue("streamPath")
	targetStreamPath := q.Get("targetStreamPath")
	if targetStreamPath == "" {
		targetStreamPath = streamPath + "/sei"
	}
	ok := conf.Server.Streams.Has(streamPath)
	if !ok {
		util.ReturnError(util.APIErrorNoStream, streamPath+" not found", w, r)
		return
	}
	var transformer *sei.Transformer
	if tm, ok := conf.Server.Transforms.Transformed.Get(targetStreamPath); ok {
		transformer, ok = tm.TransformJob.Transformer.(*sei.Transformer)
		if !ok {
			util.ReturnError(util.APIErrorPublish, "targetStreamPath is not a sei transformer", w, r)
			return
		}
	} else {
		transformer = sei.NewTransform().(*sei.Transformer)
		conf.Transform(streamPath, config.Transform{
			Output: []config.TransfromOutput{
				{
					Target:     targetStreamPath,
					StreamPath: targetStreamPath,
				},
			},
		})
	}
	t := q.Get("type")
	tb, err := strconv.ParseInt(t, 10, 8)
	if err != nil {
		if t == "" {
			tb = 5
		} else {
			util.ReturnError(util.APIErrorQueryParse, "type must a number", w, r)
			return
		}
	}
	sei, err := io.ReadAll(r.Body)
	if err != nil {
		util.ReturnError(util.APIErrorNoBody, err.Error(), w, r)
		return
	}
	transformer.AddSEI(byte(tb), sei)
	util.ReturnOK(w, r)
}
