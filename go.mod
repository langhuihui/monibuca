module github.com/langhuihui/monibuca

go 1.16

require (
	github.com/Monibuca/engine/v3 v3.3.9
	github.com/Monibuca/plugin-gateway/v3 v3.0.0-20211019071855-0dcd7beaae5b
	github.com/Monibuca/plugin-gb28181/v3 v3.0.0-20211031030004-75b1890e20d3
	github.com/Monibuca/plugin-hdl/v3 v3.0.0-20210807135828-9d98f5b8dd6c
	github.com/Monibuca/plugin-hls/v3 v3.0.0-20210821065544-cb61e2220aac
	github.com/Monibuca/plugin-jessica/v3 v3.0.0-20210807235919-48ac5fbec646
	github.com/Monibuca/plugin-logrotate/v3 v3.0.0-20210710104346-3db68431dcab
	github.com/Monibuca/plugin-record/v3 v3.0.0-20210813073316-79dce1e0dc70
	github.com/Monibuca/plugin-rtmp/v3 v3.0.0-20210819063901-526f2917b16d
	github.com/Monibuca/plugin-rtsp/v3 v3.0.0-20211014020055-a331359e972f
	github.com/Monibuca/plugin-summary v0.0.0-20210821070131-2261e0efb7b9
	github.com/Monibuca/plugin-ts/v3 v3.0.0-20210710125303-3fb5757b7c5b
	github.com/Monibuca/plugin-webrtc/v3 v3.0.0-20210817013155-6993496f6d3c
)

//replace github.com/Monibuca/plugin-gateway/v3 => ../plugin-gateway

// replace github.com/Monibuca/engine/v3 => ../engine
// replace github.com/Monibuca/plugin-summary => ../plugin-summary
