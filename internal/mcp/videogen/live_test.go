package videogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

// Explicit environment opt-in: this test creates two billable upstream jobs.
// It never reads production credentials or mutates the application's database.
func TestLiveGatewayVideoSmoke(t *testing.T) {
	endpoint, key := os.Getenv("VIDEOGEN_SMOKE_ENDPOINT"), os.Getenv("VIDEOGEN_SMOKE_API_KEY")
	textModel, imageModel := os.Getenv("VIDEOGEN_SMOKE_TEXT_MODEL"), os.Getenv("VIDEOGEN_SMOKE_IMAGE_MODEL")
	if endpoint == "" || key == "" || textModel == "" || imageModel == "" {
		t.Skip("live video endpoint/key/text model/image model are not configured")
	}
	format := os.Getenv("VIDEOGEN_SMOKE_IMAGE_FORMAT")
	if format == "" {
		format = "multipart"
	}
	imageURL := os.Getenv("VIDEOGEN_SMOKE_IMAGE_URL")
	if format == "json" && imageURL == "" {
		t.Fatal("JSON live image testing requires VIDEOGEN_SMOKE_IMAGE_URL accessible to the upstream")
	}
	seconds := float64(4)
	if s := os.Getenv("VIDEOGEN_SMOKE_SECONDS"); s != "" {
		var err error
		seconds, err = strconv.ParseFloat(s, 64)
		if err != nil || seconds <= 0 {
			t.Fatal("invalid smoke duration")
		}
	}
	size := os.Getenv("VIDEOGEN_SMOKE_SIZE")
	if size == "" {
		size = "1280x720"
	}
	if imageURL == "" {
		img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
		for y := range 720 {
			for x := range 1280 {
				img.SetRGBA(x, y, color.RGBA{R: uint8(x * 200 / 1280), G: uint8(y * 180 / 720), B: 100, A: 255})
			}
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			t.Fatal(err)
		}
		imageURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	}
	m := managerFixture(t)
	m.PollInterval = 5 * time.Second
	for _, op := range []string{"generate", "image"} {
		cfg := configFor(endpoint)
		cfg.APIKey, _ = common.Encrypt(key)
		cfg.ModelName = textModel
		cfg.RequestTimeoutSeconds = 60
		cfg.ImageRequestFormat = format
		args := map[string]any{"prompt": "A slow cinematic camera pan over abstract colored scenery, gentle motion, no people or text.", "seconds": seconds, "size": size}
		if op == "image" {
			cfg.ModelName = imageModel
			args["image"] = imageURL
		}
		job, err := m.Submit(context.Background(), 1, cfg, op, "smoke", args)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(15 * time.Minute)
		state := ""
		var row *model.VideoGenTask
		for time.Now().Before(deadline) {
			m.ProcessDue()
			row, err = m.Get(1, nil, job.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			if current := row.Status + "/" + row.ArtifactStatus; current != state {
				state = current
				t.Logf("%s model=%s task=%s %s", op, cfg.ModelName, job.ID, state)
			}
			if done(row) {
				break
			}
			time.Sleep(time.Second)
		}
		if row == nil || row.ArtifactStatus != "ready" {
			t.Fatalf("%s did not produce a saved video: status=%s error=%s last_error=%s", op, state, row.Error, row.LastError)
		}
		outputDir := filepath.Join("..", "..", "..", "tmp", "videogen-smoke")
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			t.Fatal(err)
		}
		ext := ".mp4"
		if row.MIME == "video/webm" {
			ext = ".webm"
		}
		out, err := os.Create(filepath.Join(outputDir, op+ext))
		if err != nil {
			t.Fatal(err)
		}
		reader, err := m.Store.Get(context.Background(), row.StorageKey)
		if err != nil {
			out.Close()
			t.Fatal(err)
		}
		_, err = io.Copy(out, reader)
		reader.Close()
		out.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
