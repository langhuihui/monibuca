package plugin_onvif

import (
	"github.com/kerberos-io/onvif/ptz"
	"github.com/kerberos-io/onvif/xsd"
	"github.com/kerberos-io/onvif/xsd/onvif"
)

type ImageSettings struct {
	BacklightCompensation struct {
		Mode  string `json:"Mode"`
		Level int    `json:"Level"`
	} `json:"BacklightCompensation"`
	Brightness      int `json:"Brightness"`
	ColorSaturation int `json:"ColorSaturation"`
	Contrast        int `json:"Contrast"`
	Exposure        struct {
		Mode     string `json:"Mode"`
		Priority string `json:"Priority"`
		Window   struct {
			Bottom int `json:"Bottom"`
			Top    int `json:"Top"`
			Right  int `json:"Right"`
			Left   int `json:"Left"`
		} `json:"Window"`
		MinExposureTime int `json:"MinExposureTime"`
		MaxExposureTime int `json:"MaxExposureTime"`
		MinGain         int `json:"MinGain"`
		MaxGain         int `json:"MaxGain"`
		MinIris         int `json:"MinIris"`
		MaxIris         int `json:"MaxIris"`
		ExposureTime    int `json:"ExposureTime"`
		Gain            int `json:"Gain"`
		Iris            int `json:"Iris"`
	} `json:"Exposure"`
	Focus struct {
		AutoFocusMode string `json:"AutoFocusMode"`
		DefaultSpeed  int    `json:"DefaultSpeed"`
		NearLimit     int    `json:"NearLimit"`
		FarLimit      int    `json:"FarLimit"`
		Extension     string `json:"Extension"`
	} `json:"Focus"`
	IrCutFilter      string `json:"IrCutFilter"`
	Sharpness        int    `json:"Sharpness"`
	WideDynamicRange struct {
		Mode  string `json:"Mode"`
		Level int    `json:"Level"`
	} `json:"WideDynamicRange"`
	WhiteBalance struct {
		Mode      string `json:"Mode"`
		CrGain    int    `json:"CrGain"`
		CbGain    int    `json:"CbGain"`
		Extension string `json:"Extension"`
	} `json:"WhiteBalance"`
	Extension struct {
		ImageStabilization struct {
			Mode      string `json:"Mode"`
			Level     int    `json:"Level"`
			Extension string `json:"Extension"`
		} `json:"ImageStabilization"`
		Extension struct {
			IrCutFilterAutoAdjustment struct {
				BoundaryType   string `json:"BoundaryType"`
				BoundaryOffset int    `json:"BoundaryOffset"`
				ResponseTime   string `json:"ResponseTime"`
				Extension      string `json:"Extension"`
			} `json:"IrCutFilterAutoAdjustment"`
			Extension struct {
				ToneCompensation struct {
					Mode      string `json:"Mode"`
					Level     int    `json:"Level"`
					Extension string `json:"Extension"`
				} `json:"ToneCompensation"`
				Defogging struct {
					Mode      string `json:"Mode"`
					Level     int    `json:"Level"`
					Extension string `json:"Extension"`
				} `json:"Defogging"`
				NoiseReduction struct {
					Level int `json:"Level"`
				} `json:"NoiseReduction"`
				Extension string `json:"Extension"`
			} `json:"Extension"`
		} `json:"Extension"`
	} `json:"Extension"`
}

type ImageSettingReq struct {
	ForcePersistence bool          `json:"ForcePersistence"`
	ImageSettings    ImageSettings `json:"ImageSettings"`
}

type ptzMoveReq struct {
	Move  ptz.Vector `json:"Move"`
	Speed ptz.Speed  `json:"Speed"`
}
type ptzContinueMoveReq struct {
	Velocity *onvif.PTZSpeed `json:"Velocity,omitempty"`
	Timeout  *xsd.Duration   `json:"Timeout,omitempty"`
}
