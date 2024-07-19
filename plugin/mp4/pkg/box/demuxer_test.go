package box

import (
	"fmt"
	"io"
	"os"
	"testing"
)

func TestCreateMovDemuxer(t *testing.T) {
	f, err := os.Open("source.200kbps.768x320.flv.mp4")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()
	vfile, _ := os.OpenFile("v.h264", os.O_CREATE|os.O_RDWR, 0666)
	defer vfile.Close()
	afile, _ := os.OpenFile("a.aac", os.O_CREATE|os.O_RDWR, 0666)
	defer afile.Close()
	demuxer := CreateMp4Demuxer(f)
	if infos, err := demuxer.ReadHead(); err != nil && err != io.EOF {
		fmt.Println(err)
	} else {
		fmt.Printf("%+v\n", infos)
	}
	mp4info := demuxer.GetMp4Info()
	fmt.Printf("%+v\n", mp4info)
	for {
		pkg, err := demuxer.ReadPacket()
		if err != nil {
			fmt.Println(err)
			break
		}
		fmt.Printf("track:%d,cid:%+v,pts:%d dts:%d\n", pkg.TrackId, pkg.Cid, pkg.Pts, pkg.Dts)
		if pkg.Cid == MP4_CODEC_H264 {
			vfile.Write(pkg.Data)
		} else if pkg.Cid == MP4_CODEC_AAC {
			afile.Write(pkg.Data)
		}
	}
}
