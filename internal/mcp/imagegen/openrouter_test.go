package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenRouterEndpoints(t *testing.T) {
	for input, want := range map[string]string{
		"":                                     "https://openrouter.ai/api/v1",
		"https://openrouter.ai":                "https://openrouter.ai/api/v1",
		"https://openrouter.ai///":             "https://openrouter.ai/api/v1",
		"https://openrouter.ai/api":            "https://openrouter.ai/api/v1",
		"https://openrouter.ai/api/v1/":        "https://openrouter.ai/api/v1",
		"https://proxy.example":                "https://proxy.example/v1",
		"https://proxy.example/router/api/v1/": "https://proxy.example/router/api/v1",
		"https://proxy.example/router/v1":      "https://proxy.example/router/v1",
	} {
		got, err := BaseURL(input, ProviderOpenRouter)
		if err != nil || got != want {
			t.Fatalf("%q: %s, %v", input, got, err)
		}
		for _, format := range []string{"", EditFormatMultipart, EditFormatGenerationsJSON, EditFormatGenerationsJSONExtraBody} {
			c := Client{Provider: ProviderOpenRouter, EndpointURL: input, EditRequestFormat: format}
			for _, operation := range []string{"generate", "edit"} {
				if endpoint, err := c.Endpoint(operation); err != nil || endpoint != want+"/images" {
					t.Fatalf("%s/%s: %s, %v", operation, format, endpoint, err)
				}
			}
		}
	}
}

func TestOpenRouterImageRequests(t *testing.T) {
	for _, modelName := range []string{"openai/gpt-image-2", "google/gemini-3.1-flash-image", "bytedance-seed/seedream-4.5"} {
		t.Run(modelName, func(t *testing.T) {
			calls := 0
			operation := "generate"
			inputs := []Image{{Bytes: []byte("reference-one"), MIME: "image/png"}, {Bytes: []byte("reference-two"), MIME: "image/jpeg"}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/proxy/api/v1/images" || r.Header.Get("Authorization") != "Bearer router-key" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("x-goog-api-key") != "" {
					t.Errorf("wrong protocol or authentication: %s %v", r.URL, r.Header)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body["model"] != modelName || body["prompt"] != "draw" || body["n"] != float64(2) || body["stream"] != false || body["resolution"] != "2K" || body["aspect_ratio"] != "16:9" || body["seed"] != float64(42) {
					t.Errorf("wrong body: %v", body)
				}
				if body["provider"].(map[string]any)["allow_fallbacks"] != false {
					t.Error("lost provider routing")
				}
				for _, key := range []string{"image", "images", "mask", "size", "quality", "response_format", "image_config", "messages", "return_mode", "extra_body"} {
					if _, present := body[key]; present {
						t.Errorf("unexpected wire parameter: %s", key)
					}
				}
				refs, present := body["input_references"].([]any)
				if operation == "generate" && present {
					t.Error("generation included reference images")
				}
				if operation == "edit" {
					if len(refs) != len(inputs) {
						t.Errorf("missing references: %v", refs)
						return
					}
					for i, reference := range refs {
						ref := reference.(map[string]any)
						if ref["type"] != "image_url" || ref["image_url"].(map[string]any)["url"] != "data:"+inputs[i].MIME+";base64,"+base64.StdEncoding.EncodeToString(inputs[i].Bytes) {
							t.Error("reference bytes or MIME changed")
						}
					}
				}
				fmt.Fprint(w, `{"data":[{"b64_json":"b25l","media_type":"image/webp","revised_prompt":"revised"},{"url":"https://images.example/two.png"}],"usage":{"cost":0.03}}`)
			}))
			defer server.Close()
			defaults := map[string]any{"resolution": "1K", "aspect_ratio": "1:1", "extra_body": map[string]any{"seed": float64(1), "provider": map[string]any{"allow_fallbacks": false}}}
			params, _ := MergeParams(defaults, map[string]any{"prompt": "draw", "n": float64(2), "resolution": "2K", "aspect_ratio": "16:9", "extra_body": map[string]any{"seed": float64(42)}})
			client := &Client{Provider: ProviderOpenRouter, EndpointURL: server.URL + "/proxy/api/v1", Model: modelName, APIKey: "router-key"}
			for _, op := range []string{"generate", "edit"} {
				operation = op
				result, err := client.Run(context.Background(), op, params, inputs, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Images) != 2 || string(result.Images[0].Bytes) != "one" || result.Images[0].MIME != "image/webp" || result.Images[0].RevisedPrompt != "revised" || result.Images[1].URL == "" || len(result.Usage) == 0 {
					t.Fatalf("lost output or usage: %+v", result)
				}
			}
			if calls != 2 || defaults["resolution"] != "1K" {
				t.Fatal("retried a generation or modified stored defaults")
			}
		})
	}
}

func TestOpenRouterImageModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/prefix/api/v1/images/models" || r.Header.Get("Authorization") != "Bearer router-key" {
			t.Error("wrong model catalog request")
		}
		fmt.Fprint(w, `{"data":[{"id":"bytedance-seed/seedream-4.5","name":"Seedream 4.5","supported_parameters":{"resolution":{"type":"enum","values":["1K"]}}},{"id":"custom/alias"}]}`)
	}))
	defer server.Close()
	models, err := (&Client{Provider: ProviderOpenRouter, EndpointURL: server.URL + "/prefix/api/v1", APIKey: "router-key"}).ListModels(context.Background())
	if err != nil || len(models) != 2 || models[0].Name != "Seedream 4.5" || models[1].Name != "custom/alias" {
		t.Fatalf("model IDs were filtered or capabilities became name restrictions: %+v %v", models, err)
	}
}

func TestOpenRouterImageValidationAndErrors(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		switch body["prompt"] {
		case "slow":
			<-r.Context().Done()
		case "empty":
			fmt.Fprint(w, `{"data":[]}`)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":{"message":"router-key unavailable"}}`)
		}
	}))
	defer server.Close()
	client := &Client{Provider: ProviderOpenRouter, EndpointURL: server.URL, APIKey: "router-key", Model: "manual/alias"}
	for _, invalid := range []map[string]any{
		{"n": float64(0)}, {"n": 1.5}, {"n": float64(11)}, {"image_size": "2K"}, {"resolution": 2048}, {"output_format": "svg"},
		{"extra_body": map[string]any{"INPUT_REFERENCES": []any{}}},
		{"extra_body": map[string]any{"models": []any{"different/model"}}},
		{"extra_body": map[string]any{"stream": true}},
		{"extra_body": map[string]any{"extra_body": map[string]any{"input_references": []any{}}}},
	} {
		invalid["prompt"] = "draw"
		if _, err := client.Run(context.Background(), "generate", invalid, nil, nil); err == nil || calls != 0 {
			t.Fatalf("invalid parameter reached upstream: %v, %v", invalid, err)
		}
	}
	image := Image{Bytes: []byte("image"), MIME: "image/png"}
	if _, err := client.Run(context.Background(), "edit", map[string]any{"prompt": "draw"}, []Image{image}, &image); err == nil || !strings.Contains(err.Error(), "mask") || calls != 0 {
		t.Fatal("unsupported mask reached upstream")
	}
	if _, err := client.Run(context.Background(), "edit", map[string]any{"prompt": "draw"}, nil, nil); err == nil || calls != 0 {
		t.Fatal("missing references reached upstream")
	}
	if _, err := client.Run(context.Background(), "generate", map[string]any{"prompt": "draw"}, nil, nil); err == nil || strings.Contains(err.Error(), "router-key") || !strings.Contains(err.Error(), "503") || calls != 1 {
		t.Fatalf("unexpected retry or credential exposure: %v", err)
	}
	if _, err := client.Run(context.Background(), "generate", map[string]any{"prompt": "empty"}, nil, nil); err == nil || !strings.Contains(err.Error(), "no images") {
		t.Fatal("empty image response accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := client.Run(ctx, "generate", map[string]any{"prompt": "slow"}, nil, nil); err == nil || !strings.Contains(err.Error(), "deadline") || calls != 3 {
		t.Fatalf("cancellation or retry behavior: %v", err)
	}
}
