package plugin_sei

import (
	"context"
	"errors"

	globalPB "m7s.live/v5/pb"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	pb "m7s.live/v5/plugin/sei/pb"
	sei "m7s.live/v5/plugin/sei/pkg"
)

func (conf *SEIPlugin) Insert(ctx context.Context, req *pb.InsertRequest) (*globalPB.SuccessResponse, error) {
	streamPath := req.StreamPath
	targetStreamPath := req.TargetStreamPath
	if targetStreamPath == "" {
		targetStreamPath = streamPath + "/sei"
	}
	ok := conf.Server.Streams.Has(streamPath)
	if !ok {
		return nil, pkg.ErrNotFound
	}
	var transformer *sei.Transformer
	if tm, ok := conf.Server.Transforms.Get(targetStreamPath); ok {
		transformer, ok = tm.TransformJob.Transformer.(*sei.Transformer)
		if !ok {
			return nil, errors.New("targetStreamPath is not a sei transformer")
		}
	} else {
		transformer = sei.NewTransform().(*sei.Transformer)
		transformer.TransformJob.Init(transformer, &conf.Plugin, streamPath, config.Transform{
			Output: []config.TransfromOutput{
				{
					Target:     targetStreamPath,
					StreamPath: targetStreamPath,
				},
			},
		}).WaitStarted()
	}
	t := req.Type

	transformer.AddSEI(byte(t), req.Data)
	err := transformer.WaitStarted()
	if err != nil {
		return nil, err
	}
	return &globalPB.SuccessResponse{
		Code:    0,
		Message: "success",
	}, nil
}
