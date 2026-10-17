package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/videogen"
	"github.com/mujkjk/newmcp/model"
)

func TestOpenRouterVideoConfigAndTools(t *testing.T) {
	db := imageGenTestSetup(t)
	if err := db.AutoMigrate(&model.VideoGenConfig{}); err != nil {
		t.Fatal(err)
	}
	s := &VideoGenService{}
	detail, err := s.Create(1, &dto.VideoGenConfigReq{
		Name: igPtr("OpenRouter video"), Protocol: igPtr(videogen.ProtocolOpenRouter), ModelName: igPtr("heygen/heygen-video-1"),
		EndpointURL: igPtr("https://openrouter.ai"), APIKey: igPtr("secret-video-key"),
		DefaultParams: igPtr(json.RawMessage(`{"seconds":6,"extra_body":{"resolution":"768p","aspect_ratio":"16:9"}}`)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Protocol != videogen.ProtocolOpenRouter || detail.ImageRequestFormat != "json" || detail.GenerateVideoDescDefault != model.DefaultOpenRouterGenerateVideoDesc || detail.GenerateVideoDesc != detail.GenerateVideoDescDefault {
		t.Fatal("OpenRouter defaults did not match protocol", detail)
	}
	if err := s.Enable(1, detail.ID); err != nil {
		t.Fatal(err)
	}
	cfg, _ := model.GetVideoGenConfig(1, detail.ID)
	svc, _ := model.GetServiceByID(1, *cfg.RegisteredServiceID)
	var tools []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Schema      struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"inputSchema"`
	}
	if err := json.Unmarshal([]byte(svc.ToolsCache), &tools); err != nil {
		t.Fatal(err)
	}
	if len(tools) != 4 || tools[0].Schema.Properties["seconds"]["type"] != "integer" || tools[0].Schema.Properties["metadata"] != nil || tools[0].Description != model.DefaultOpenRouterGenerateVideoDesc || tools[1].Description != model.DefaultOpenRouterGenerateVideoFromImageDesc {
		t.Fatal("OpenRouter registered tools mismatch", tools)
	}
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), "secret-video-key") || strings.Contains(string(raw), `"api_key"`) {
		t.Fatal("config leaked key")
	}
	if err := s.Update(1, detail.ID, &dto.VideoGenConfigReq{Protocol: igPtr(videogen.ProtocolOpenAI), GenerateVideoFromImageDesc: igPtr("Custom image description")}); err != nil {
		t.Fatal(err)
	}
	detail, _ = s.Get(1, detail.ID)
	if detail.GenerateVideoDesc != model.DefaultGenerateVideoDesc || detail.GenerateVideoFromImageDesc != "Custom image description" || detail.GenerateVideoDescDefault != model.DefaultGenerateVideoDesc {
		t.Fatal("protocol switch lost default/custom description", detail)
	}
	if err := s.Update(1, detail.ID, &dto.VideoGenConfigReq{Protocol: igPtr(videogen.ProtocolOpenRouter)}); err != nil {
		t.Fatal(err)
	}
	detail, _ = s.Get(1, detail.ID)
	if detail.GenerateVideoDesc != model.DefaultOpenRouterGenerateVideoDesc || detail.GenerateVideoFromImageDesc != "Custom image description" {
		t.Fatal("custom description overwritten")
	}
	for _, params := range []string{`{"seconds":5.5}`, `{"metadata":{"resolution":"720p"}}`, `{"seconds":6,"extra_body":{"duration":6}}`, `{"extra_body":{"previous_job_id":"other-task"}}`} {
		if err := s.Update(1, detail.ID, &dto.VideoGenConfigReq{DefaultParams: igPtr(json.RawMessage(params))}); err == nil {
			t.Fatal("invalid protocol defaults accepted", params)
		}
	}
}

func TestOpenRouterVideoDraftAndSavedModels(t *testing.T) {
	db := imageGenTestSetup(t)
	if err := db.AutoMigrate(&model.VideoGenConfig{}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/proxy/api/v1/videos/models" || r.Header.Get("Authorization") != "Bearer secret-video-key" {
			t.Error("unexpected catalog request", r.Method, r.URL)
		}
		io.WriteString(w, `{"data":[{"id":"heygen/heygen-video-1"},{"id":"minimax/hailuo-3-max"},{"id":"google/veo-3.1-lite"}]}`)
	}))
	defer server.Close()
	s := &VideoGenService{}
	r := &dto.VideoGenProbeReq{VideoGenConfigReq: dto.VideoGenConfigReq{Protocol: igPtr(videogen.ProtocolOpenRouter), EndpointURL: igPtr(server.URL + "/proxy/api/v1"), APIKey: igPtr("secret-video-key")}}
	models, err := s.Models(context.Background(), 1, r)
	if err != nil || len(models) != 3 {
		t.Fatalf("draft models: %v %v", models, err)
	}
	var count int64
	db.Model(&model.VideoGenConfig{}).Count(&count)
	if count != 0 {
		t.Fatal("draft discovery saved configuration")
	}
	r.Name, r.ModelName = igPtr("Saved OpenRouter"), igPtr("google/veo-3.1-lite")
	detail, err := s.Create(1, &r.VideoGenConfigReq)
	if err != nil {
		t.Fatal(err)
	}
	models, err = s.Models(context.Background(), 1, &dto.VideoGenProbeReq{ConfigID: detail.ID, VideoGenConfigReq: dto.VideoGenConfigReq{Name: igPtr(""), ModelName: igPtr(""), APIKey: igPtr("")}})
	if err != nil || len(models) != 3 {
		t.Fatalf("saved models: %v %v", models, err)
	}
	if _, err := s.Models(context.Background(), 2, &dto.VideoGenProbeReq{ConfigID: detail.ID}); err == nil {
		t.Fatal("cross-user discovery accepted")
	}
}

func TestVideoGenOfficialEndpointDefaults(t *testing.T) {
	db := imageGenTestSetup(t)
	if err := db.AutoMigrate(&model.VideoGenConfig{}); err != nil {
		t.Fatal(err)
	}
	s := &VideoGenService{}
	for protocol, endpoint := range map[string]string{
		videogen.ProtocolOpenAI:     videogen.DefaultOpenAIEndpoint,
		videogen.ProtocolGemini:     videogen.DefaultGeminiEndpoint,
		videogen.ProtocolOpenRouter: videogen.DefaultOpenRouterEndpoint,
	} {
		t.Run(protocol, func(t *testing.T) {
			detail, err := s.Create(1, &dto.VideoGenConfigReq{Name: igPtr("Official video"), Protocol: igPtr(protocol), ModelName: igPtr("video-model"), APIKey: igPtr("test-key")})
			if err != nil || detail.EndpointURL != endpoint {
				t.Fatalf("omitted address default: detail=%+v err=%v", detail, err)
			}
			custom := "https://gateway.example/proxy/v1"
			if err := s.Update(1, detail.ID, &dto.VideoGenConfigReq{EndpointURL: &custom}); err != nil {
				t.Fatal(err)
			}
			if err := s.Update(1, detail.ID, &dto.VideoGenConfigReq{Name: igPtr("Renamed video")}); err != nil {
				t.Fatal(err)
			}
			detail, _ = s.Get(1, detail.ID)
			if detail.EndpointURL != custom {
				t.Fatal("omitted update replaced custom address")
			}
			if err := s.Update(1, detail.ID, &dto.VideoGenConfigReq{EndpointURL: igPtr("  ")}); err != nil {
				t.Fatal(err)
			}
			detail, _ = s.Get(1, detail.ID)
			if detail.EndpointURL != endpoint {
				t.Fatal("clearing address did not restore protocol default")
			}
			cfg, err := s.probe(1, &dto.VideoGenProbeReq{VideoGenConfigReq: dto.VideoGenConfigReq{Name: igPtr("Draft video"), Protocol: igPtr(protocol), ModelName: igPtr("video-model"), APIKey: igPtr("test-key"), EndpointURL: igPtr("")}})
			if err != nil || cfg.EndpointURL != endpoint {
				t.Fatalf("blank draft address: config=%+v err=%v", cfg, err)
			}
		})
	}
}

func TestGeminiVideoConfigAndTools(t *testing.T) {
	db := imageGenTestSetup(t)
	if err := db.AutoMigrate(&model.VideoGenConfig{}); err != nil {
		t.Fatal(err)
	}
	s := &VideoGenService{}
	detail, err := s.Create(1, &dto.VideoGenConfigReq{
		Name: igPtr("Gemini video"), Protocol: igPtr(videogen.ProtocolGemini), ModelName: igPtr("veo-3.1-generate-preview"), APIKey: igPtr("native-secret"),
		DefaultParams: igPtr(json.RawMessage(`{"seconds":8,"size":"720p","extra_body":{"aspectRatio":"16:9"}}`)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if detail.EndpointURL != videogen.DefaultGeminiEndpoint || detail.ImageRequestFormat != "json" || detail.GenerateVideoDesc != model.DefaultGeminiGenerateVideoDesc || detail.GenerateVideoFromImageDescDefault != model.DefaultGeminiGenerateVideoFromImageDesc {
		t.Fatal("wrong Gemini defaults", detail)
	}
	if err := s.Enable(1, detail.ID); err != nil {
		t.Fatal(err)
	}
	cfg, _ := model.GetVideoGenConfig(1, detail.ID)
	svc, _ := model.GetServiceByID(1, *cfg.RegisteredServiceID)
	var tools []struct {
		Description string `json:"description"`
		Schema      struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"inputSchema"`
	}
	if err := json.Unmarshal([]byte(svc.ToolsCache), &tools); err != nil {
		t.Fatal(err)
	}
	if len(tools) != 4 || tools[0].Schema.Properties["seconds"]["type"] != "integer" || tools[0].Schema.Properties["metadata"] != nil || !strings.Contains(tools[0].Schema.Properties["size"]["description"].(string), "resolution") || tools[1].Description != model.DefaultGeminiGenerateVideoFromImageDesc {
		t.Fatal("wrong Gemini MCP schema", tools)
	}
	for _, protocol := range []string{videogen.ProtocolOpenRouter, videogen.ProtocolOpenAI, videogen.ProtocolGemini} {
		if err := s.Update(1, detail.ID, &dto.VideoGenConfigReq{Protocol: igPtr(protocol), DefaultParams: igPtr(json.RawMessage(`{}`)), GenerateVideoFromImageDesc: igPtr("Keep my custom description")}); err != nil {
			t.Fatal(err)
		}
		detail, _ = s.Get(1, detail.ID)
		if detail.GenerateVideoDesc != detail.GenerateVideoDescDefault || detail.GenerateVideoFromImageDesc != "Keep my custom description" {
			t.Fatal("protocol switch lost default/custom descriptions", detail)
		}
	}
	for _, params := range []string{`{"metadata":{"mode":"text"}}`, `{"seconds":5.5}`, `{"extra_body":{"numberOfVideos":2}}`, `{"extra_body":{"referenceImages":[]}}`} {
		if err := s.Update(1, detail.ID, &dto.VideoGenConfigReq{DefaultParams: igPtr(json.RawMessage(params))}); err == nil {
			t.Fatal("invalid Gemini defaults accepted", params)
		}
	}
}

func TestGeminiVideoDraftAndSavedModels(t *testing.T) {
	db := imageGenTestSetup(t)
	if err := db.AutoMigrate(&model.VideoGenConfig{}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/v1beta/models" || r.Header.Get("x-goog-api-key") != "native-secret" || r.Header.Get("Authorization") != "" {
			t.Error("invalid native models request", r.URL)
		}
		io.WriteString(w, `{"models":[{"name":"models/veo-test","supportedGenerationMethods":["predictLongRunning"]}]}`)
	}))
	defer server.Close()
	s := &VideoGenService{}
	r := &dto.VideoGenProbeReq{VideoGenConfigReq: dto.VideoGenConfigReq{Protocol: igPtr(videogen.ProtocolGemini), EndpointURL: igPtr(server.URL + "/proxy"), APIKey: igPtr("native-secret")}}
	models, err := s.Models(context.Background(), 1, r)
	if err != nil || len(models) != 1 || models[0].ID != "veo-test" {
		t.Fatalf("draft model discovery: %v %v", models, err)
	}
	r.Name, r.ModelName = igPtr("Saved Gemini"), igPtr("veo-test")
	detail, err := s.Create(1, &r.VideoGenConfigReq)
	if err != nil {
		t.Fatal(err)
	}
	models, err = s.Models(context.Background(), 1, &dto.VideoGenProbeReq{ConfigID: detail.ID, VideoGenConfigReq: dto.VideoGenConfigReq{APIKey: igPtr("")}})
	if err != nil || len(models) != 1 {
		t.Fatalf("saved model discovery: %v %v", models, err)
	}
	if _, err := s.Models(context.Background(), 2, &dto.VideoGenProbeReq{ConfigID: detail.ID}); err == nil {
		t.Fatal("cross-user native model discovery accepted")
	}
}
