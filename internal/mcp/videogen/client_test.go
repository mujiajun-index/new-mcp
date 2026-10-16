package videogen

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
)

func TestBaseURLAndParameters(t *testing.T) {
	for _, base := range []string{"https://gateway.example", "https://gateway.example/v1/", "https://gateway.example/proxy/v1"} {
		got, err := BaseURL(base)
		if err != nil || strings.Count(got, "/v1") != 1 {
			t.Fatalf("%s: %s %v", base, got, err)
		}
	}
	for _, base := range []string{"file:///etc/passwd", "https://user:secret@gateway.example", "https://gateway.example?q=secret", "https://gateway.example#secret"} {
		if _, err := BaseURL(base); err == nil {
			t.Fatalf("accepted %s", base)
		}
	}
	for _, params := range []map[string]any{{"extra_body": map[string]any{"MODEL": "override"}}, {"extra_body": map[string]any{"input_reference": "override"}}, {"extra_body": map[string]any{"headers": map[string]any{}}}, {"wait_seconds": float64(31)}, {"metadata": []any{}}, {"seconds": float64(0)}} {
		if err := ValidateParams(params, false); err == nil {
			t.Fatalf("accepted invalid params %v", params)
		}
	}
	if err := ValidateParams(map[string]any{"seconds": float64(7.5), "metadata": map[string]any{"resolution": "4k"}}, true); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitSerializationAndResponse(t *testing.T) {
	for _, format := range []string{"json", "multipart"} {
		t.Run(format, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/proxy/v1/videos" || r.Header.Get("Authorization") != "Bearer upstream-secret" {
					t.Errorf("bad request %s", r.URL.Path)
				}
				if format == "json" {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if !strings.HasPrefix(body["input_reference"].(string), "data:image/png;base64,") || body["seed"] != float64(42) {
						t.Error("missing JSON reference or extension")
					}
					if _, ok := body["wait_seconds"]; ok {
						t.Error("local control leaked upstream")
					}
				} else {
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Fatal(err)
					}
					defer r.MultipartForm.RemoveAll()
					var metadata map[string]any
					if json.Unmarshal([]byte(r.FormValue("metadata")), &metadata) != nil || metadata["resolution"] != "720p" {
						t.Error("invalid metadata serialization")
					}
					file, header, err := r.FormFile("input_reference")
					if err != nil {
						t.Fatal(err)
					}
					defer file.Close()
					data, _ := io.ReadAll(file)
					if !bytes.Equal(data, []byte("png-bytes")) || header.Header.Get("Content-Type") != "image/png" {
						t.Error("invalid image file")
					}
				}
				io.WriteString(w, `{"task_id":"gateway-public-task","status":"queued","progress":"12%"}`)
			}))
			defer server.Close()
			client := &Client{Connection: Connection{EndpointURL: server.URL + "/proxy/v1/", APIKey: "upstream-secret", Model: "arbitrary-video-model"}}
			params := map[string]any{"prompt": "motion", "seconds": float64(7.5), "metadata": map[string]any{"resolution": "720p"}, "extra_body": map[string]any{"seed": 42}, "wait_seconds": float64(0)}
			req, err := client.BuildSubmit(context.Background(), params, &imagegen.Image{MIME: "image/png", Bytes: []byte("png-bytes")}, format)
			if err != nil {
				t.Fatal(err)
			}
			v, err := client.Submit(req)
			if err != nil || v.ID != "gateway-public-task" || v.ProgressValue() != 12 {
				t.Fatalf("%+v %v", v, err)
			}
		})
	}
}

func TestUpstreamErrorRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(429)
		io.WriteString(w, `{"error":{"message":"bad upstream-secret"}}`)
	}))
	defer server.Close()
	c := &Client{Connection: Connection{EndpointURL: server.URL, APIKey: "upstream-secret"}}
	_, err := c.Query(context.Background(), "id")
	if err == nil || strings.Contains(err.Error(), "upstream-secret") {
		t.Fatal("credential leak")
	}
	he, ok := err.(*HTTPError)
	if !ok || he.RetryAfter.Seconds() != 2 {
		t.Fatalf("%v", err)
	}
	if message := c.redact(strings.Repeat("x", 4090) + "upstream-secret" + strings.Repeat("y", 5000)); len(message) > 4100 || strings.Contains(message, "upstream-secret") {
		t.Fatal("upstream error was not bounded and redacted")
	}
}

// If the reference gateway and Node are present, execute its actual protocol
// decoders against wire requests built by this adapter. CI without reference
// sources still runs the serialization and response tests above.
func TestNewAPIGatewayPluginCompatibility(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	plugins := filepath.Join(root, "reference", "new-api-main", "plugins", "tasks")
	if _, err := os.Stat(plugins); err != nil {
		t.Skip("reference gateway not available")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node not available")
	}
	cases := []struct {
		key, model string
		seconds    float64
	}{{"sora", "sora-2", 4}, {"google", "veo-3.1-generate-preview", 8}, {"vertex-ai", "veo-3.1-generate-preview", 8}, {"alibaba", "wan2.6-i2v", 5}, {"doubao", "doubao-seedance-1-0-pro-250528", 5}, {"kling", "kling-v1", 5}, {"hailuo", "MiniMax-Hailuo-02", 6}, {"jimeng", "jimeng_vgfm_t2v_l20", 5}, {"vidu", "viduq2", 4}, {"xai", "grok-imagine-video", 5}}
	fixtures := []map[string]any{}
	for _, tc := range cases {
		for _, mode := range []string{"text", "json", "multipart"} {
			modelName := tc.model
			if tc.key == "alibaba" && mode == "text" {
				modelName = "wan2.6-t2v"
			}
			c := &Client{Connection: Connection{EndpointURL: "https://gateway.example", Model: tc.model, APIKey: "fixture-key"}}
			c.Model = modelName
			params := map[string]any{"prompt": "A slow camera pan over a landscape", "seconds": tc.seconds, "metadata": map[string]any{}}
			var img *imagegen.Image
			if mode != "text" {
				img = &imagegen.Image{MIME: "image/png", Bytes: []byte("fixture-image")}
				if mode == "json" {
					img.URL = "https://fixtures.example/reference.png"
				}
			}
			req, err := c.BuildSubmit(context.Background(), params, img, mode)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if mode == "multipart" {
				if err := req.ParseMultipartForm(1 << 20); err != nil {
					t.Fatal(err)
				}
				body = map[string]any{"kind": "multipart", "fields": req.PostForm, "files": []any{map[string]any{"field": "input_reference", "ref": "request_file:input_reference", "filename": "reference.png", "mimeType": "image/png", "size": 13}}}
				req.MultipartForm.RemoveAll()
			} else {
				var value map[string]any
				if err := json.NewDecoder(req.Body).Decode(&value); err != nil {
					t.Fatal(err)
				}
				body = map[string]any{"kind": "json", "value": value}
			}
			fixture := map[string]any{"key": tc.key, "model": modelName, "mode": mode, "body": body}
			if tc.key == "alibaba" && mode == "multipart" {
				fixture["expected_error"] = "Alibaba requires image references to be URLs"
			}
			if tc.key == "doubao" && mode == "multipart" {
				fixture["expected_error"] = "Doubao requires image and video references to be URLs inside metadata.content"
			}
			fixtures = append(fixtures, fixture)
		}
	}
	file := filepath.Join(t.TempDir(), "requests.json")
	data, _ := json.Marshal(fixtures)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, filepath.Join(root, "scripts", "check-video-gateway-protocols.mjs"), plugins, file)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gateway protocol compatibility: %v\n%s", err, output)
	}
	t.Log(string(output))
}
