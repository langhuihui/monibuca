package plugin_webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"m7s.live/v5"
)

func TestPublish(t *testing.T) {
	ctx, cancel := chromedp.NewContext(context.Background())
	go m7s.Run(ctx, "config.yaml")
	defer cancel()
	err := chromedp.Run(ctx,
		chromedp.Navigate("http://localhost:8080/webrtc/test/publish"),
	)
	if err != nil {
		t.Fatal(err)
	}
	<-time.After(10 * time.Second)
}
