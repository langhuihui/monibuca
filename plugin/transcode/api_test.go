package plugin_transcode

import (
	"testing"
)

func Test_parseCoordinates(t *testing.T) {
	type args struct {
		coordString string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "test1",
			args: args{
				coordString: "100,100",
			},
			want: "x=100:y=100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCoordinates(tt.args.coordString)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCoordinates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseCoordinates() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseCrop(t *testing.T) {
	type args struct {
		cropString string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "test1",
			args: args{
				cropString: "100,100,200,200",
			},
			want: "crop=200:200:100:100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCrop(tt.args.cropString)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCrop() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseCrop() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_rgbToHex(t *testing.T) {
	type args struct {
		FontColor string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "test1",
			args: args{
				FontColor: "255,255,255",
			},
			want: ":fontcolor=#ffffff",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFontColor(tt.args.FontColor)
			if (err != nil) != tt.wantErr {
				t.Errorf("rgbToHex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("rgbToHex() got = %v, want %v", got, tt.want)
			}
		})
	}
}
