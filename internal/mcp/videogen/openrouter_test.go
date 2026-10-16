package videogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/model"
)

var openRouterModelFixtures = []string{"heygen/heygen-video-1", "minimax/hailuo-3-max", "google/veo-3.1-lite"}

func TestOpenRouterBaseURL(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		for _, raw := range []string{"", " \t "} {
			if got, err := BaseURLForProtocol(raw, protocol); err != nil || got != DefaultEndpoint(protocol) {
				t.Fatalf("empty %s endpoint: %q, %v", protocol, got, err)
			}
		}
	}
	for raw, want := range map[string]string{
		"https://openrouter.ai":                 "https://openrouter.ai/api/v1",
		"https://openrouter.ai/":                "https://openrouter.ai/api/v1",
		"https://openrouter.ai/api":             "https://openrouter.ai/api/v1",
		"https://openrouter.ai/api/v1/":         "https://openrouter.ai/api/v1",
		"https://gateway.example/proxy/api/v1/": "https://gateway.example/proxy/api/v1",
		"https://gateway.example/proxy/v1":      "https://gateway.example/proxy/v1",
		"https://gateway.example":               "https://gateway.example/v1",
	} {
		if got, err := BaseURLForProtocol(raw, ProtocolOpenRouter); err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := BaseURLForProtocol("https://openrouter.ai", "unsupported"); err == nil {
		t.Fatal("accepted unsupported protocol")
	}
	if got, _ := BaseURLForProtocol("https://openrouter.ai", ""); got != "https://openrouter.ai/v1" {
		t.Fatal("legacy protocol was silently switched", got)
	}
}

func TestOpenRouterModelRequests(t *testing.T) {
	for _, modelName := range openRouterModelFixtures {
		for _, mode := range []string{"text", "image_data", "image_url"} {
			t.Run(modelName+"/"+mode, func(t *testing.T) {
				c := &Client{Connection: Connection{Protocol: ProtocolOpenRouter, EndpointURL: "https://openrouter.ai", Model: modelName, APIKey: "secret"}}
				resolution := "768p"
				if strings.HasPrefix(modelName, "google/") {
					resolution = "720p"
				}
				p := map[string]any{"prompt": "A cinematic landscape", "seconds": float64(6), "metadata": map[string]any{}, "wait_seconds": float64(0), "request_id": "local-only", "extra_body": map[string]any{"resolution": resolution, "aspect_ratio": "16:9", "seed": float64(42)}}
				if strings.HasPrefix(modelName, "google/") {
					p["extra_body"].(map[string]any)["generate_audio"] = true
					p["extra_body"].(map[string]any)["provider"] = map[string]any{"options": map[string]any{"google-vertex": map[string]any{"parameters": map[string]any{"negativePrompt": "blurry"}}}}
				}
				var img *imagegen.Image
				if mode != "text" {
					img = &imagegen.Image{MIME: "image/png", Bytes: []byte("validated-png")}
					if mode == "image_url" {
						img.URL = "https://fixtures.example/reference.png"
					}
				}
				// OpenRouter always uses JSON, even if an old caller passes multipart.
				req, err := c.BuildSubmit(context.Background(), p, img, "multipart")
				if err != nil {
					t.Fatal(err)
				}
				if req.URL.String() != "https://openrouter.ai/api/v1/videos" || req.Header.Get("Authorization") != "Bearer secret" || req.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("unexpected request: %v %v", req.URL, req.Header)
				}
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["model"] != modelName || body["duration"] != float64(6) || body["resolution"] != resolution {
					t.Fatal("missing protocol fields", body)
				}
				if strings.HasPrefix(modelName, "google/") && (body["generate_audio"] != true || body["provider"] == nil) {
					t.Fatal("missing provider extensions", body)
				}
				for _, field := range []string{"seconds", "metadata", "input_reference", "wait_seconds", "request_id", "extra_body"} {
					if _, exists := body[field]; exists {
						t.Fatal("unexpected field", field)
					}
				}
				if img == nil {
					if body["frame_images"] != nil {
						t.Fatal("text request acquired reference")
					}
				} else {
					frames := body["frame_images"].([]any)
					frame := frames[0].(map[string]any)
					want := img.URL
					if want == "" {
						want = "data:image/png;base64," + base64.StdEncoding.EncodeToString(img.Bytes)
					}
					if len(frames) != 1 || frame["type"] != "image_url" || frame["frame_type"] != "first_frame" || frame["image_url"].(map[string]any)["url"] != want {
						t.Fatal("invalid first frame", frames)
					}
				}
			})
		}
	}
}

func TestOpenRouterParameterValidation(t *testing.T) {
	c := &Client{Connection: Connection{Protocol: ProtocolOpenRouter, EndpointURL: "https://openrouter.ai", Model: openRouterModelFixtures[0]}}
	for _, p := range []map[string]any{
		{"seconds": float64(5.5)},
		{"seconds": float64(6), "extra_body": map[string]any{"duration": float64(6)}},
		{"extra_body": map[string]any{"duration": "6"}},
		{"extra_body": map[string]any{"duration": float64(-1)}},
		{"extra_body": map[string]any{"duration": float64(1.5)}},
		{"metadata": map[string]any{"resolution": "720p"}},
		{"extra_body": map[string]any{"frame_images": []any{}}},
		{"extra_body": map[string]any{"INPUT_REFERENCES": []any{}}},
		{"extra_body": map[string]any{"previous_job_id": "someone-elses-job"}},
		{"extra_body": map[string]any{"PREVIOUS_JOB_ID": "someone-elses-job"}},
	} {
		p["prompt"] = "scene"
		if _, err := c.BuildSubmit(context.Background(), p, nil, "json"); err == nil {
			t.Fatalf("accepted invalid OpenRouter parameters: %v", p)
		}
	}
	p := map[string]any{"prompt": "scene", "extra_body": map[string]any{"duration": float64(6), "resolution": "768p"}}
	if _, err := c.BuildSubmit(context.Background(), p, nil, "json"); err != nil {
		t.Fatal("valid native duration rejected", err)
	}
}

func TestVideoModelCatalogByProtocol(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		t.Run(protocol, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				want := "/proxy/v1/models"
				if protocol == ProtocolOpenRouter {
					want = "/proxy/v1/videos/models"
				}
				if r.URL.Path != want || r.Header.Get("Authorization") != "Bearer secret" {
					t.Errorf("unexpected model discovery request %s", r.URL)
				}
				io.WriteString(w, `{"data":[{"id":"heygen/heygen-video-1","name":"HeyGen: Video 1"},{"id":"minimax/hailuo-3-max"},{"id":"google/veo-3.1-lite","supported_durations":[4,6,8]}]}`)
			}))
			defer server.Close()
			c := &Client{Connection: Connection{Protocol: protocol, EndpointURL: server.URL + "/proxy/v1", APIKey: "secret"}}
			models, err := c.ListModels(context.Background())
			if err != nil || len(models) != 3 || models[1].Name != models[1].ID {
				t.Fatalf("models=%+v err=%v", models, err)
			}
		})
	}
}

func TestOpenRouterTasksResumeAndSave(t *testing.T) {
	for _, modelName := range openRouterModelFixtures {
		for _, operation := range []string{"generate", "image"} {
			t.Run(modelName+"/"+operation, func(t *testing.T) {
				m := managerFixture(t)
				var posts, polls, downloads atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer fixture-secret" {
						t.Error("missing auth")
					}
					switch {
					case r.Method == http.MethodPost && r.URL.Path == "/api/v1/videos":
						posts.Add(1)
						var body map[string]any
						json.NewDecoder(r.Body).Decode(&body)
						if body["model"] != modelName || body["duration"] != float64(6) || (operation == "image" && body["frame_images"] == nil) {
							t.Error("invalid submit", body)
						}
						w.WriteHeader(http.StatusAccepted)
						io.WriteString(w, `{"id":"openrouter-job","status":"pending","polling_url":"https://untrusted.example/steal"}`)
					case r.URL.Path == "/api/v1/videos/openrouter-job":
						polls.Add(1)
						// The content endpoint works without unsigned_urls in the reply.
						io.WriteString(w, `{"id":"openrouter-job","status":"completed"}`)
					case r.URL.Path == "/api/v1/videos/openrouter-job/content" && r.URL.RawQuery == "index=0":
						downloads.Add(1)
						w.Header().Set("Content-Type", "video/mp4")
						w.Write(mp4Bytes())
					default:
						t.Errorf("unexpected endpoint %s %s", r.Method, r.URL)
						w.WriteHeader(404)
					}
				}))
				defer server.Close()
				cfg := configFor(server.URL + "/api/v1")
				cfg.Protocol, cfg.ModelName, cfg.ImageRequestFormat = ProtocolOpenRouter, modelName, "json"
				args := map[string]any{"prompt": "scene", "seconds": float64(6), "request_id": "once"}
				if operation == "image" {
					var pngData bytes.Buffer
					png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2)))
					args["image"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData.Bytes())
				}
				// ServerAddress is deliberately unset: OpenRouter sends data URLs.
				job, err := m.Submit(context.Background(), 1, cfg, operation, "test", args)
				if err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(3 * time.Second)
				var accepted *model.VideoGenTask
				for time.Now().Before(deadline) {
					accepted, _ = m.Get(1, nil, job.ID, false)
					if accepted.UpstreamID != "" {
						break
					}
					time.Sleep(time.Millisecond)
				}
				if accepted.UpstreamID == "" {
					t.Fatal("submission not accepted", accepted)
				}
				plain, _ := common.Decrypt(accepted.Connection)
				var snapshot Connection
				json.Unmarshal([]byte(plain), &snapshot)
				if snapshot.Protocol != ProtocolOpenRouter || accepted.Protocol != ProtocolOpenRouter {
					t.Fatal("protocol missing from task or snapshot")
				}
				m.Close()
				// Editing the configuration cannot change an accepted task's protocol.
				cfg.Protocol, cfg.EndpointURL = ProtocolOpenAI, "https://invalid.example"
				restarted := NewManager(m.DB, m.Store)
				restarted.PollInterval = time.Millisecond
				defer restarted.Close()
				row := awaitTask(t, restarted, job.ID, func(row *model.VideoGenTask) bool { return row.ArtifactStatus == "ready" })
				if posts.Load() != 1 || polls.Load() != 1 || downloads.Load() != 1 || row.Connection != "" || TaskView(row)["protocol"] != ProtocolOpenRouter {
					t.Fatalf("post=%d polls=%d downloads=%d row=%+v", posts.Load(), polls.Load(), downloads.Load(), row)
				}
				raw, _ := MCPResult(row)
				if !strings.Contains(string(raw), "resource_link") || strings.Contains(string(raw), "fixture-secret") {
					t.Fatal("missing saved resource or leaked credential")
				}
			})
		}
	}
}

func TestOpenRouterExpiredTaskFails(t *testing.T) {
	m := managerFixture(t)
	values := m.videoValues(&model.VideoGenTask{}, &Client{}, &Video{Status: "expired", Error: json.RawMessage(`"Job exceeded maximum time to live"`)})
	if values["status"] != "failed" || values["connection"] != "" || values["stopped_at"] == nil {
		t.Fatal("expired job was not terminal", values)
	}
}

func TestOpenRouterRejectsUnsafeInputBeforeSubmission(t *testing.T) {
	m := managerFixture(t)
	for _, operation := range []string{"generate", "image"} {
		args := map[string]any{"prompt": "scene", "extra_body": map[string]any{"previous_job_id": "unowned-job"}}
		cfg := configFor("https://openrouter.ai")
		cfg.Protocol = ProtocolOpenRouter
		if _, err := m.Submit(context.Background(), 1, cfg, operation, "test", args); err == nil || !strings.Contains(err.Error(), "previous_job_id") {
			t.Fatal("continuation bypass accepted", operation, err)
		}
	}
	for _, ref := range []string{"http://127.0.0.1/private.png", "http://169.254.169.254/latest/meta-data", "data:text/html;base64,PGh0bWw+"} {
		cfg := configFor("https://openrouter.ai")
		cfg.Protocol = ProtocolOpenRouter
		if _, err := m.Submit(context.Background(), 1, cfg, "image", "test", map[string]any{"prompt": "scene", "image": ref}); err == nil {
			t.Fatal("unsafe image accepted", ref)
		}
	}
	var count int64
	m.DB.Model(&model.VideoGenTask{}).Count(&count)
	if count != 0 {
		t.Fatal("invalid input persisted a generation", count)
	}
}
