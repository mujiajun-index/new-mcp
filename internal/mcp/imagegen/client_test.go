package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBaseURL(t *testing.T) {
	for _, tc := range []struct{ input, provider, want string }{
		{"https://gateway.example", "openai", "https://gateway.example/v1"},
		{"https://gateway.example/proxy/v1/", "openai", "https://gateway.example/proxy/v1"},
		{"https://gateway.example/proxy/v1beta", "openai", "https://gateway.example/proxy/v1"},
		{"https://gateway.example/proxy/v1/", "gemini", "https://gateway.example/proxy/v1beta"},
		{"https://gateway.example/v1beta///", "gemini", "https://gateway.example/v1beta"},
	} {
		got, err := BaseURL(tc.input, tc.provider)
		if err != nil || got != tc.want {
			t.Fatalf("%s: %s, %v", tc.input, got, err)
		}
	}
	for _, bad := range []string{"file:///tmp/foo", "https://user:pass@gateway.example", "https://gateway.example?key=secret", "https://gateway.example#foo"} {
		if _, err := BaseURL(bad, "openai"); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	c := Client{Provider: "gemini", EndpointURL: "https://gateway.example/prefix/v1beta", Model: "models/gemini-image"}
	if got, _ := c.Endpoint("edit"); got != "https://gateway.example/prefix/v1beta/models/gemini-image:generateContent" {
		t.Fatal(got)
	}
}

func TestOpenAICompatibleGatewayModels(t *testing.T) {
	// New API model families all use the same public image wire format, without
	// name heuristics or adapter-specific size/quality enums in the MCP layer.
	for _, name := range []string{"gpt-image-1", "qwen-image-plus", "doubao-seedream-4-0", "imagen-4.0-generate-001", "custom-image-alias"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/proxy/v1/images/generations" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("bad request: %s %v", r.URL, r.Header)
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["model"] != name || body["prompt"] != "draw" || body["n"] != float64(1) || body["size"] != "2K" || body["quality"] != "gateway-quality" {
					t.Errorf("body = %v", body)
				}
				if body["seed"] != float64(42) || body["watermark"] != false || body["prompt_extend"] != true {
					t.Errorf("gateway parameters lost: %v", body)
				}
				for _, k := range []string{"response_format", "background", "output_format", "stream", "extra_body", "return_mode"} {
					if _, ok := body[k]; ok {
						t.Errorf("sent unset/internal %s", k)
					}
				}
				fmt.Fprint(w, `{"data":[{"b64_json":"cG5n","revised_prompt":"revised"},{"url":"https://cdn.example/image.png"}],"usage":{"images":2}}`)
			}))
			defer server.Close()
			var defaults, args map[string]any
			_ = json.Unmarshal([]byte(`{"size":"1024x1024","extra_body":{"seed":1,"watermark":false}}`), &defaults)
			_ = json.Unmarshal([]byte(`{"prompt":"draw","size":"2K","quality":"gateway-quality","extra_body":{"seed":42,"prompt_extend":true}}`), &args)
			params, _ := MergeParams(defaults, args)
			result, err := (&Client{Provider: "openai", EndpointURL: server.URL + "/proxy/v1", Model: name, APIKey: "test-key"}).Run(context.Background(), "generate", params, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Images) != 2 || string(result.Images[0].Bytes) != "png" || result.Images[0].RevisedPrompt != "revised" || result.Images[1].URL == "" {
				t.Fatalf("result: %+v", result)
			}
			if defaults["size"] != "1024x1024" || defaults["extra_body"].(map[string]any)["seed"] != float64(1) {
				t.Fatal("mutated configuration defaults")
			}
		})
	}
}

func TestOpenAIEditMultipart(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/images/edits" || r.Header.Get("Authorization") != "Bearer secret" {
					t.Errorf("bad route/auth")
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					return
				}
				defer r.MultipartForm.RemoveAll()
				field := "image"
				if count > 1 {
					field = "image[]"
				}
				if len(r.MultipartForm.File[field]) != count || len(r.MultipartForm.File["mask"]) != 1 {
					t.Errorf("files: %v", r.MultipartForm.File)
				}
				for _, file := range r.MultipartForm.File[field] {
					if file.Header.Get("Content-Type") != "image/png" {
						t.Error(file.Header)
					}
				}
				if r.FormValue("model") != "qwen-image-edit" || r.FormValue("n") != "1" || r.FormValue("seed") != "99" {
					t.Error(r.MultipartForm.Value)
				}
				fmt.Fprint(w, `{"data":{"b64_json":"b2s="}}`)
			}))
			defer server.Close()
			images := make([]Image, count)
			for i := range images {
				images[i] = Image{Bytes: []byte("png"), MIME: "image/png"}
			}
			result, err := (&Client{Provider: "openai", EndpointURL: server.URL, Model: "qwen-image-edit", APIKey: "secret"}).Run(context.Background(), "edit", map[string]any{"prompt": "edit", "extra_body": map[string]any{"seed": 99}}, images, &images[0])
			if err != nil || string(result.Images[0].Bytes) != "ok" {
				t.Fatalf("%v %v", result, err)
			}
		})
	}
}

func TestGenerationsJSONEditing(t *testing.T) {
	for _, format := range []string{EditFormatGenerationsJSON, EditFormatGenerationsJSONExtraBody} {
		for _, count := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/%d", format, count), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Header.Get("Authorization") != "Bearer secret" {
						t.Error("missing gateway authentication")
					}
					if r.URL.Path == "/proxy/v1/images/edits" {
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(w, `{"error":{"message":"OpenAIException - no available server"}}`)
						return
					}
					if r.URL.Path != "/proxy/v1/images/generations" || r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("wrong editing protocol: %s, %s", r.URL.Path, r.Header.Get("Content-Type"))
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					if body["model"] != "agnes-image-2.1-flash" || body["prompt"] != "edit reference" || body["n"] != float64(1) || body["size"] != "1K" || body["seed"] != float64(42) {
						t.Errorf("lost model/parameters: %v", body)
					}
					imageBody := body
					if format == EditFormatGenerationsJSONExtraBody {
						if _, exists := body["image"]; exists {
							t.Error("nested JSON format sent a top-level image")
						}
						imageBody, _ = body["extra_body"].(map[string]any)
						if imageBody["response_format"] != "url" {
							t.Error("lost native nested extension")
						}
					}
					refs, ok := imageBody["image"].([]any)
					if !ok || len(refs) != count {
						t.Errorf("lost references: %v", imageBody)
					} else {
						for _, ref := range refs {
							if ref != "data:image/png;base64,cG5n" {
								t.Errorf("not forwarding image bytes as data URLs: %v", ref)
							}
						}
					}
					for _, k := range []string{"images", "mask", "response_format", "return_mode", "stream"} {
						if _, exists := body[k]; exists {
							t.Errorf("unexpected top-level field: %s", k)
						}
					}
					fmt.Fprint(w, `{"data":[{"b64_json":"b2s="}]}`)
				}))
				defer server.Close()
				images := make([]Image, count)
				for i := range images {
					images[i] = Image{Bytes: []byte("png"), MIME: "image/png", URL: "http://localhost:3000/u/private?s=signature"}
				}
				client := Client{Provider: "openai", EndpointURL: server.URL + "/proxy/v1", Model: "agnes-image-2.1-flash", APIKey: "secret"}
				params := map[string]any{"prompt": "edit reference", "size": "1K", "return_mode": "url", "extra_body": map[string]any{"seed": 42}}
				if _, err := client.Run(context.Background(), "edit", params, images, nil); err == nil || !strings.Contains(err.Error(), "HTTP 503") || calls != 1 {
					t.Fatalf("standard edits failure was not preserved or was retried: calls=%d, %v", calls, err)
				}
				client.EditRequestFormat = format
				if format == EditFormatGenerationsJSONExtraBody {
					params["extra_body"].(map[string]any)["extra_body"] = map[string]any{"response_format": "url"}
				}
				result, err := client.Run(context.Background(), "edit", params, images, nil)
				if err != nil || len(result.Images) != 1 || string(result.Images[0].Bytes) != "ok" || calls != 2 {
					t.Fatalf("JSON edit failed: calls=%d, %+v, %v", calls, result, err)
				}
				if _, exists := params["extra_body"].(map[string]any)["image"]; exists {
					t.Fatal("mutated caller parameters")
				}
				if _, err := client.Run(context.Background(), "edit", params, images, &images[0]); err == nil || !strings.Contains(err.Error(), "mask") || calls != 2 {
					t.Fatal("JSON mask was sent upstream", err)
				}
			})
		}
	}
	for _, tc := range []struct{ provider, format string }{{"openai", "unknown"}, {"gemini", EditFormatGenerationsJSON}} {
		if err := ValidateEditRequestFormat(tc.provider, tc.format); err == nil {
			t.Fatalf("accepted incompatible edit format: %+v", tc)
		}
	}
}

func TestGeminiNativeEditAndMixedCandidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gw/v1beta/models/gemini-image:generateContent" || r.Header.Get("x-goog-api-key") != "secret" || r.Header.Get("Authorization") != "" {
			t.Errorf("bad route/auth: %s %v", r.URL, r.Header)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gen := body["generationConfig"].(map[string]any)
		img := gen["imageConfig"].(map[string]any)
		if img["aspectRatio"] != "16:9" || img["imageSize"] != "2K" || gen["temperature"] != 0.4 || len(body["contents"].([]any)[0].(map[string]any)["parts"].([]any)) != 2 {
			t.Error(body)
		}
		for _, k := range []string{"model", "n", "aspect_ratio", "image_size"} {
			if _, ok := body[k]; ok {
				t.Errorf("sent %s", k)
			}
		}
		fmt.Fprintf(w, `{"candidates":[{"content":{"parts":[{"text":"first"},{"inlineData":{"mimeType":"image/png","data":"%s"}}]}},{"content":{"parts":[{"text":"second"},{"inline_data":{"mime_type":"image/jpeg","data":"%s"}}]}}],"usageMetadata":{"tokens":12}}`, base64.StdEncoding.EncodeToString([]byte("a")), base64.StdEncoding.EncodeToString([]byte("b")))
	}))
	defer server.Close()
	params := map[string]any{"prompt": "edit", "aspect_ratio": "16:9", "image_size": "2K", "extra_body": map[string]any{"generationConfig": map[string]any{"temperature": 0.4}}}
	result, err := (&Client{Provider: "gemini", EndpointURL: server.URL + "/gw/v1", Model: "gemini-image", APIKey: "secret"}).Run(context.Background(), "edit", params, []Image{{Bytes: []byte("ref"), MIME: "image/png"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 2 || result.Text != "first\nsecond" || result.Images[1].MIME != "image/jpeg" {
		t.Fatalf("%+v", result)
	}
}

func TestImageParametersAndUpstreamFailure(t *testing.T) {
	for _, raw := range []string{`{"extra_body":{"model":"x"}}`, `{"extra_body":{"stream":true}}`, `{"extra_body":{"n":20}}`, `{"n":11}`, `{"n":1.5}`, `{"size":2048}`, `{"extra_body":{"contents":[]}}`, `{"extra_body":{"generationConfig":{"response_modalities":["TEXT"]}}}`, `{"extra_body":{"extra_body":{"image":["override"]}}}`, `{"extra_body":{"extra_body":{"MODEL":"override"}}}`, `{"extra_body":{"extra_body":{"stream":true}}}`, `{"extra_body":{"extra_body":[]}}`} {
		var p map[string]any
		_ = json.Unmarshal([]byte(raw), &p)
		if ValidateParams("openai", p, false) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if _, err := parseOpenAI([]byte(`{"data":[]}`)); err == nil {
		t.Fatal("accepted no images")
	}
	if _, err := parseGemini([]byte(`{"candidates":[{"content":{"parts":[{"text":"refused"}]}}]}`)); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"bad secret-key"}}`)
	}))
	defer server.Close()
	_, err := (&Client{Provider: "openai", EndpointURL: server.URL, Model: "gpt-image", APIKey: "secret-key"}).Run(context.Background(), "generate", map[string]any{"prompt": "x"}, nil, nil)
	if err == nil || strings.Contains(err.Error(), "secret-key") || calls != 1 {
		t.Fatalf("calls %d error %v", calls, err)
	}
}

func TestImageRequestCancellationAndModelList(t *testing.T) {
	canceled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			fmt.Fprint(w, `{"data":[{"id":"chat-model"},{"id":"unknown-image-alias"}]}`)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
			canceled <- struct{}{}
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()
	client := Client{Provider: "openai", EndpointURL: server.URL, Model: "slow", APIKey: "k"}
	models, err := client.ListModels(context.Background())
	if err != nil || len(models) != 2 {
		t.Fatalf("filtered models: %v %v", models, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := client.Run(ctx, "generate", map[string]any{"prompt": "x"}, nil, nil); err == nil {
		t.Fatal("ignored deadline")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream context not canceled")
	}
}
