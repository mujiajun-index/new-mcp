package videogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/model"
)

func TestGeminiVideoEndpointsAndSubmit(t *testing.T) {
	for raw, want := range map[string]string{
		"": DefaultGeminiEndpoint,
		" https://generativelanguage.googleapis.com/ ": DefaultGeminiEndpoint,
		"https://proxy.example/gemini":                 "https://proxy.example/gemini/v1beta",
		"https://proxy.example/gemini/v1/":             "https://proxy.example/gemini/v1beta",
		"https://proxy.example/gemini/v1beta/":         "https://proxy.example/gemini/v1beta",
	} {
		if got, err := BaseURLForProtocol(raw, ProtocolGemini); err != nil || got != want {
			t.Fatalf("base %q: %s %v", raw, got, err)
		}
	}
	for _, modelName := range []string{"veo-3.1-generate-preview", "models/veo-3.1-lite-generate-preview"} {
		for _, withImage := range []bool{false, true} {
			c := &Client{Connection: Connection{Protocol: ProtocolGemini, APIKey: "gemini-secret", Model: modelName}}
			p := map[string]any{"prompt": "A cat running", "seconds": float64(8), "size": "720p", "wait_seconds": float64(0), "request_id": "local-only", "extra_body": map[string]any{"aspectRatio": "16:9", "negativePrompt": "blur", "seed": float64(42), "numberOfVideos": float64(1)}}
			var img *imagegen.Image
			if withImage {
				img = &imagegen.Image{MIME: "image/png", Bytes: []byte("validated-image"), URL: "https://private-store.example/signed"}
			}
			req, err := c.BuildSubmit(context.Background(), p, img, "multipart")
			if err != nil {
				t.Fatal(err)
			}
			if req.URL.String() != DefaultGeminiEndpoint+"/models/"+strings.TrimPrefix(modelName, "models/")+":predictLongRunning" || req.Header.Get("x-goog-api-key") != "gemini-secret" || req.Header.Get("Authorization") != "" || req.URL.RawQuery != "" || req.Header.Get("Content-Type") != "application/json" {
				t.Fatal("wrong native endpoint or credentials", req.URL, req.Header)
			}
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			instance := body["instances"].([]any)[0].(map[string]any)
			parameters := body["parameters"].(map[string]any)
			if len(body) != 2 || instance["prompt"] != p["prompt"] || parameters["durationSeconds"] != float64(8) || parameters["resolution"] != "720p" || parameters["aspectRatio"] != "16:9" || parameters["seed"] != float64(42) || parameters["sampleCount"] != float64(1) || parameters["numberOfVideos"] != nil || parameters["negativePrompt"] != "blur" {
				t.Fatal("incorrect native mapping", body)
			}
			if withImage {
				inline := instance["image"].(map[string]any)
				if inline["mimeType"] != "image/png" || inline["bytesBase64Encoded"] != base64.StdEncoding.EncodeToString(img.Bytes) || inline["inlineData"] != nil {
					t.Fatal("image bytes not embedded", inline)
				}
			} else if instance["image"] != nil {
				t.Fatal("text generation acquired an image")
			}
		}
	}
}

func TestGeminiVideoRejectsUnsafeOrAmbiguousParams(t *testing.T) {
	c := &Client{Connection: Connection{Protocol: ProtocolGemini, Model: "veo-3.1-generate-preview"}}
	for _, p := range []map[string]any{
		{"seconds": float64(5.5)},
		{"metadata": map[string]any{"resolution": "720p"}},
		{"seconds": float64(8), "extra_body": map[string]any{"durationSeconds": float64(8)}},
		{"size": "720p", "extra_body": map[string]any{"resolution": "720p"}},
		{"extra_body": map[string]any{"durationSeconds": "8"}},
		{"extra_body": map[string]any{"durationSeconds": float64(1.5)}},
		{"extra_body": map[string]any{"numberOfVideos": float64(2)}},
		{"extra_body": map[string]any{"sampleCount": float64(2)}},
		{"extra_body": map[string]any{"SAMPLE_COUNT": float64(2)}},
	} {
		p["prompt"] = "scene"
		if _, err := c.BuildSubmit(context.Background(), p, nil, "json"); err == nil {
			t.Fatal("accepted invalid parameters", p)
		}
	}
	for _, key := range []string{"instances", "parameters", "image", "MODEL", "video", "referenceImages", "reference_images", "lastFrame", "storageUri", "output_gcs_uri", "httpOptions", "api_key", "request_id"} {
		if _, err := c.BuildSubmit(context.Background(), map[string]any{"prompt": "scene", "extra_body": map[string]any{key: "untrusted"}}, nil, "json"); err == nil {
			t.Fatal("accepted protected field", key)
		}
	}
	for _, modelName := range []string{"../secret", "https://other.example/model", "models/foo/operations/bar", "veo?key=secret", ".", ".."} {
		c.Model = modelName
		if _, err := c.BuildSubmit(context.Background(), map[string]any{"prompt": "scene"}, nil, "json"); err == nil {
			t.Fatal("accepted invalid model path", modelName)
		}
	}
	c.Model = "models/veo-new-model"
	if _, err := c.BuildSubmit(context.Background(), map[string]any{"prompt": "scene", "extra_body": map[string]any{"durationSeconds": float64(8), "resolution": "new-resolution", "aspectRatio": "new-ratio"}}, nil, "json"); err != nil {
		t.Fatal("imposed a model/parameter whitelist", err)
	}
}

func TestGeminiOperationStates(t *testing.T) {
	for _, tc := range []struct{ name, raw, status, message string }{
		{"pending", `{"name":"models/veo/operations/job"}`, "in_progress", ""},
		{"running", `{"name":"operations/job","done":false}`, "in_progress", ""},
		{"success", `{"name":"operations/job","done":true,"response":{"generateVideoResponse":{"generatedSamples":[{"video":{"uri":"https://example.com/file.mp4"}}]}}}`, "completed", ""},
		{"failed", `{"name":"operations/job","done":true,"error":{"code":3,"message":"invalid prompt"}}`, "failed", "invalid prompt"},
		{"filtered", `{"name":"operations/job","done":true,"response":{"generateVideoResponse":{"raiMediaFilteredCount":1,"raiMediaFilteredReasons":["Policy reason"]}}}`, "failed", "Policy reason"},
		{"empty", `{"name":"operations/job","done":true,"response":{}}`, "failed", "without a generated video"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op, err := parseGeminiOperation([]byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			v := op.video()
			if v.Status != tc.status || !strings.Contains(string(v.Error), tc.message) {
				t.Fatal("incorrect operation status", v)
			}
		})
	}
	for _, raw := range []string{`null`, `{`, `{"name":"https://evil.example/steal"}`, `{"name":"models/../operations/job"}`, `{"name":"operations/%2e%2e"}`} {
		if _, err := parseGeminiOperation([]byte(raw)); err == nil {
			t.Fatal("accepted invalid operation", raw)
		}
	}
	for _, name := range []string{"../secret", "https://evil.example", "operations/job?key=secret", "models/veo/operations/../secret"} {
		if _, err := (&Client{Connection: Connection{Protocol: ProtocolGemini}}).Query(context.Background(), name); err == nil {
			t.Fatal("accepted arbitrary polling path", name)
		}
	}
}

func TestGeminiVideoCatalogPaginationAndQueries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("x-goog-api-key") != "secret" || r.Header.Get("Authorization") != "" || r.URL.Query().Get("key") != "" {
			t.Error("incorrect catalog authentication")
		}
		if r.URL.Path == "/proxy/v1beta/models" {
			if r.URL.Query().Get("pageToken") == "" {
				io.WriteString(w, `{"models":[{"name":"models/veo-one","displayName":"Veo One","supportedGenerationMethods":["predictLongRunning"]},{"name":"models/gemini-text","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"next +/?"}`)
			} else if r.URL.Query().Get("pageToken") == "next +/?" {
				io.WriteString(w, `{"models":[{"name":"models/veo-two","supportedGenerationMethods":["predictLongRunning"]},{"name":"models/veo-one","supportedGenerationMethods":["predictLongRunning"]}]}`)
			} else {
				t.Error("page token was not encoded")
			}
			return
		}
		if r.URL.Path == "/proxy/v1beta/models/veo-one/operations/job" {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":{"code":429,"message":"quota secret"}}`)
			return
		}
		t.Error("unexpected Gemini route", r.URL)
	}))
	defer server.Close()
	c := &Client{Connection: Connection{Protocol: ProtocolGemini, EndpointURL: server.URL + "/proxy", APIKey: "secret"}}
	models, err := c.ListModels(context.Background())
	if err != nil || len(models) != 2 || models[0].ID != "veo-one" || models[0].Name != "Veo One" || models[1].Name != "veo-two" || calls.Load() != 2 {
		t.Fatalf("models=%+v err=%v calls=%d", models, err, calls.Load())
	}
	if _, err := c.Query(context.Background(), "models/veo-one/operations/job"); err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "429") {
		t.Fatal("query error not handled/redacted", err)
	}
}

func TestGeminiTasksRecoverAndRetrySaving(t *testing.T) {
	for _, operation := range []string{"generate", "image"} {
		t.Run(operation, func(t *testing.T) {
			m := managerFixture(t)
			var posts, downloads, polls atomic.Int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-goog-api-key") != "fixture-secret" || r.Header.Get("Authorization") != "" {
					t.Error("missing native authentication", r.URL)
				}
				switch r.URL.Path {
				case "/native/v1beta/models/veo-test:predictLongRunning":
					posts.Add(1)
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					instance := body["instances"].([]any)[0].(map[string]any)
					if r.Method != http.MethodPost || instance["prompt"] != "scene" || (operation == "image" && instance["image"] == nil) {
						t.Error("invalid native submission", body)
					}
					io.WriteString(w, `{"name":"models/veo-test/operations/job"}`)
				case "/native/v1beta/models/veo-test/operations/job":
					if polls.Add(1) == 1 {
						w.WriteHeader(http.StatusTooManyRequests)
						io.WriteString(w, `{"error":{"code":429,"message":"try later"}}`)
						return
					}
					fmt.Fprintf(w, `{"name":"models/veo-test/operations/job","done":true,"response":{"generateVideoResponse":{"generatedSamples":[{"video":{"uri":%q}}]}}}`, server.URL+"/download/v1beta/files/movie:download?alt=media")
				case "/download/v1beta/files/movie:download":
					if downloads.Add(1) == 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.Write(mp4Bytes())
				default:
					t.Error("unexpected route", r.URL)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			cfg := configFor(server.URL + "/native")
			cfg.Protocol, cfg.ModelName, cfg.ImageRequestFormat = ProtocolGemini, "veo-test", "json"
			// Draft tasks and inline images must work without ServerAddress.
			if operation == "image" {
				cfg.ID = 0
			}
			args := map[string]any{"prompt": "scene", "seconds": float64(8), "request_id": "once"}
			if operation == "image" {
				var data bytes.Buffer
				png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2)))
				args["image"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
			}
			job, err := m.Submit(context.Background(), 1, cfg, operation, "test", args)
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				job, _ = m.Get(1, nil, job.ID, false)
				if job.UpstreamID != "" {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if job.UpstreamID != "models/veo-test/operations/job" {
				t.Fatal("operation name was not persisted", job)
			}
			repeated, err := m.Submit(context.Background(), 1, cfg, operation, "test", args)
			if err != nil || repeated.ID != job.ID {
				t.Fatal("idempotent native submission changed row", repeated, err)
			}
			m.Close()
			cfg.Protocol, cfg.EndpointURL = ProtocolOpenAI, "https://invalid.example"
			restarted := NewManager(m.DB, m.Store)
			restarted.PollInterval = time.Millisecond
			defer restarted.Close()
			failedSave := awaitTask(t, restarted, job.ID, func(row *model.VideoGenTask) bool { return row.ArtifactStatus == "failed" })
			if failedSave.Status != "completed" || failedSave.Connection == "" {
				t.Fatal("download failure lost generated operation", failedSave)
			}
			wrongConfig := cfg.ID + 100
			if _, err := restarted.Get(2, nil, job.ID, false); err == nil {
				t.Fatal("cross-user access accepted")
			}
			if _, err := restarted.RetryDownload(1, &wrongConfig, job.ID); err == nil {
				t.Fatal("cross-config retry accepted")
			}
			if _, err := restarted.RetryDownload(1, nil, job.ID); err != nil {
				t.Fatal(err)
			}
			ready := awaitTask(t, restarted, job.ID, func(row *model.VideoGenTask) bool { return row.ArtifactStatus == "ready" })
			var count int64
			m.DB.Model(&model.VideoGenTask{}).Count(&count)
			raw, _ := MCPResult(ready)
			if posts.Load() != 1 || downloads.Load() != 2 || count != 1 || ready.Connection != "" || ready.Protocol != ProtocolGemini || !strings.Contains(string(raw), "resource_link") || strings.Contains(string(raw), "fixture-secret") || strings.Contains(string(raw), "models/veo-test/operations/job") {
				t.Fatalf("wrong lifecycle: posts=%d downloads=%d count=%d response=%s", posts.Load(), downloads.Load(), count, raw)
			}
		})
	}
}

func TestGeminiFailedGenerationRetriesSameTask(t *testing.T) {
	m := managerFixture(t)
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "fixture-secret" {
			t.Error("missing key")
		}
		if r.Method == http.MethodPost {
			if posts.Add(1) == 1 {
				io.WriteString(w, `{"name":"operations/failed","done":true,"error":{"code":3,"message":"bad prompt"}}`)
			} else {
				io.WriteString(w, `{"name":"operations/retried"}`)
			}
			return
		}
		io.WriteString(w, `{"name":"operations/retried","done":false}`)
	}))
	defer server.Close()
	cfg := configFor(server.URL)
	cfg.Protocol, cfg.ModelName = ProtocolGemini, "veo-test"
	job, err := m.Submit(context.Background(), 1, cfg, "generate", "test", map[string]any{"prompt": "bad", "request_id": "initial"})
	if err != nil {
		t.Fatal(err)
	}
	failed := awaitTask(t, m, job.ID, func(row *model.VideoGenTask) bool { return row.Status == "failed" })
	if failed.Connection != "" || !strings.Contains(failed.Error, "bad prompt") {
		t.Fatal("terminal error handling failed", failed)
	}
	retried, err := m.RetryGeneration(context.Background(), 1, job.ID, cfg, map[string]any{"prompt": "corrected scene"}, "retry-once")
	if err != nil || retried.ID != job.ID {
		t.Fatal("retry changed task ID", retried, err)
	}
	ready := awaitTask(t, m, job.ID, func(row *model.VideoGenTask) bool { return row.UpstreamID == "operations/retried" })
	var count int64
	m.DB.Model(&model.VideoGenTask{}).Count(&count)
	if count != 1 || posts.Load() != 2 || ready.RetryCount != 1 || !ready.CreatedAt.Equal(failed.CreatedAt) || ready.Prompt != "corrected scene" {
		t.Fatal("retry did not reuse original row", ready, count, posts.Load())
	}
}
