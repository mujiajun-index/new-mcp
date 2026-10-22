package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/audiogen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func audioServiceSetup(t *testing.T) {
	db := imageGenTestSetup(t)
	if e := db.AutoMigrate(&model.AudioGenConfig{}, &model.AudioGenConnection{}, &model.AudioGenModel{}, &model.AudioGenTask{}, &model.AudioAsset{}); e != nil {
		t.Fatal(e)
	}
	old := virtual.AudioManager
	manager := audiogen.NewManager(db, UploadStore)
	virtual.AudioManager = manager
	t.Cleanup(func() { manager.Close(); virtual.AudioManager = old })
}

func TestAudioConnectionProtocolControlsRoutingAndDiscovery(t *testing.T) {
	audioServiceSetup(t)
	var discoveries []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		discoveries = append(discoveries, r.URL.Query().Get("output_modalities"))
		fmt.Fprint(w, `{"data":[{"id":"discovered-model"}]}`)
	}))
	defer upstream.Close()
	req := audioServiceReq()
	(*req.Connections)[0].Protocol = "openrouter"
	(*req.Connections)[0].BaseURL = upstream.URL
	s := &AudioGenService{}
	cfg, err := s.Create(1, req)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connections[0].Protocol != "openrouter" {
		t.Fatal("connection protocol was not stored")
	}
	for _, slot := range cfg.Models {
		want := "openrouter_chat_audio"
		modality := "audio"
		if slot.Kind == "speech" {
			want = "openrouter_speech"
			modality = "speech"
		}
		if slot.Protocol != want {
			t.Fatalf("%s routed to %s", slot.Kind, slot.Protocol)
		}
		if _, err = s.Models(context.Background(), 1, &dto.AudioGenProbeReq{ConfigID: cfg.ID, Kind: slot.Kind}); err != nil {
			t.Fatal(err)
		}
		if len(discoveries) == 0 || discoveries[len(discoveries)-1] != modality {
			t.Fatal("discovery did not follow the connection", discoveries)
		}
	}
	if len(discoveries) != 3 {
		t.Fatal("discovery did not follow the connection", discoveries)
	}
	if err = s.Enable(1, cfg.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := model.GetAudioGenConfig(1, cfg.ID)
	connections := []dto.AudioGenConnectionReq{{Key: "main", Protocol: "openai", BaseURL: upstream.URL}}
	if err = s.Update(1, cfg.ID, &dto.AudioGenConfigReq{Connections: &connections}); err == nil {
		t.Fatal("OpenAI channel accepted music")
	}
	unchanged, _ := model.GetAudioGenConfig(1, cfg.ID)
	if unchanged.Connections[0].Protocol != "openrouter" {
		t.Fatal("unsupported channel update was not atomic")
	}
	connections[0].Protocol = "gemini"
	updatedModels := []dto.AudioGenModelReq{(*req.Models)[0], (*req.Models)[2]}
	if err = s.Update(1, cfg.ID, &dto.AudioGenConfigReq{Connections: &connections, Models: &updatedModels}); err != nil {
		t.Fatal(err)
	}
	after, _ := model.GetAudioGenConfig(1, cfg.ID)
	if *before.RegisteredServiceID != *after.RegisteredServiceID || after.Connections[0].APIKey == "" {
		t.Fatal("channel update lost service identity or credentials")
	}
	for _, slot := range after.Models {
		if slot.Protocol != "gemini_generate_content" {
			t.Fatal("slot retained an old protocol", slot)
		}
	}
	// Native OpenAI channels automatically choose Speech and Chat Audio as well.
	openAI := audioServiceReq()
	(*openAI.Connections)[0].Protocol = "openai"
	models := (*openAI.Models)[:2]
	openAI.Models = &models
	native, err := s.Create(1, openAI)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range native.Models {
		want := "openai_chat_audio"
		if slot.Kind == "speech" {
			want = "openai_speech"
		}
		if slot.Protocol != want {
			t.Fatal("incorrect channel routing", slot)
		}
	}
	if _, err = s.Test(context.Background(), 1, &dto.AudioGenProbeReq{AudioGenConfigReq: *openAI, Kind: "speech", Arguments: json.RawMessage(`{"text":"draft channel test","wait_seconds":0}`)}); err != nil {
		t.Fatal(err)
	}
	var task model.AudioGenTask
	if err = model.DB.Where("user_id = ? AND kind = ?", 1, "speech").Last(&task).Error; err != nil || task.Protocol != "openai_speech" {
		t.Fatal("unsaved draft test did not snapshot the resolved channel protocol", task.Protocol, err)
	}
}

func TestAudioConnectionDefaultURLsAndCredentialRetention(t *testing.T) {
	audioServiceSetup(t)
	s := &AudioGenService{}
	connections := []dto.AudioGenConnectionReq{}
	for _, channel := range audiogen.ChannelProtocols {
		connections = append(connections, dto.AudioGenConnectionReq{Key: channel, Protocol: channel, BaseURL: " \t ", APIKey: igPtr("default-channel-key")})
	}
	models := []dto.AudioGenModelReq{{Kind: "speech", Enabled: true, ConnectionKey: "openai", Model: "tts-model"}}
	cfg, err := s.Create(1, &dto.AudioGenConfigReq{Name: igPtr("Default URLs"), Connections: &connections, Models: &models})
	if err != nil {
		t.Fatal(err)
	}
	for _, connection := range cfg.Connections {
		if connection.BaseURL != audiogen.DefaultChannelURL(connection.Protocol) || !connection.HasAPIKey {
			t.Fatal("blank URL did not use the channel default", connection)
		}
	}
	for i := range connections {
		connections[i].APIKey = nil
	}
	if err = s.Update(1, cfg.ID, &dto.AudioGenConfigReq{Connections: &connections}); err != nil {
		t.Fatal("clearing an already-default URL should keep its credentials", err)
	}
	stored, _ := model.GetAudioGenConfig(1, cfg.ID)
	for _, connection := range stored.Connections {
		if connection.APIKey == "" || connection.BaseURL == "" {
			t.Fatal("default URL or credentials were lost")
		}
	}
	musicConnections := []dto.AudioGenConnectionReq{{Key: "music", Protocol: "minimax", APIKey: igPtr("music-key")}}
	catalog, err := s.Models(context.Background(), 1, &dto.AudioGenProbeReq{AudioGenConfigReq: dto.AudioGenConfigReq{Connections: &musicConnections}, Kind: "music", ConnectionKey: "music"})
	if err != nil || len(catalog) != 1 || catalog[0].ID != "music-3.0" {
		t.Fatal("draft model discovery rejected the default URL", catalog, err)
	}
}
func audioServiceReq() *dto.AudioGenConfigReq {
	connections := []dto.AudioGenConnectionReq{{Key: "main", Protocol: "openrouter", BaseURL: "https://gateway.example/v1", APIKey: igPtr("secret-audio-key")}}
	models := []dto.AudioGenModelReq{{Kind: "speech", Enabled: true, ConnectionKey: "main", Model: "tts-model"}, {Kind: "audio", Enabled: true, ConnectionKey: "main", Model: "openai/gpt-audio"}, {Kind: "music", Enabled: true, ConnectionKey: "main", Model: "lyria-3-pro-preview"}}
	return &dto.AudioGenConfigReq{Name: igPtr("Audio suite"), Connections: &connections, Models: &models}
}
func TestAudioConfigurationLifecycleAndConditionalTools(t *testing.T) {
	audioServiceSetup(t)
	s := &AudioGenService{}
	detail, e := s.Create(1, audioServiceReq())
	if e != nil {
		t.Fatal(e)
	}
	if detail.AutoRegister || len(detail.Models) != 3 || !detail.Connections[0].HasAPIKey {
		t.Fatal(detail)
	}
	b, _ := json.Marshal(detail)
	if strings.Contains(string(b), "secret-audio-key") || strings.Contains(string(b), `"api_key"`) {
		t.Fatal("key leaked")
	}
	if _, e = s.Get(2, detail.ID); e == nil {
		t.Fatal("cross-user configuration read")
	}
	if e = s.Enable(1, detail.ID); e != nil {
		t.Fatal(e)
	}
	cfg, e := model.GetAudioGenConfig(1, detail.ID)
	if e != nil {
		t.Fatal(e)
	}
	sid := *cfg.RegisteredServiceID
	tools := buildAudioGenTools(cfg)
	if len(tools) != 6 {
		t.Fatalf("expected six tools: %v", tools)
	}
	if e = s.Enable(1, detail.ID); e != nil {
		t.Fatal(e)
	}
	again, _ := model.GetAudioGenConfig(1, detail.ID)
	if *again.RegisteredServiceID != sid {
		t.Fatal("enable changed service identity")
	}
	models := *audioServiceReq().Models
	models[1].Enabled = false
	models[2].ConnectionKey = "songs"
	models[2].Model = "music-3.0"
	connections := append(*audioServiceReq().Connections, dto.AudioGenConnectionReq{Key: "songs", Protocol: "minimax", BaseURL: "https://music.example/v1", APIKey: igPtr("music-key")})
	if e = s.Update(1, detail.ID, &dto.AudioGenConfigReq{Name: igPtr("Updated suite"), Connections: &connections, Models: &models}); e != nil {
		t.Fatal(e)
	}
	cfg, _ = model.GetAudioGenConfig(1, detail.ID)
	if *cfg.RegisteredServiceID != sid || len(buildAudioGenTools(cfg)) != 3 {
		t.Fatal("conditional tools or service ID wrong")
	}
	ctx := virtual.WithCallerUserID(context.Background(), 1)
	if _, e = VirtualRegistry.Handle(ctx, sid, nil, "generate_audio", json.RawMessage(`{"prompt":"old cached tool"}`)); e == nil {
		t.Fatal("disabled cached tool accepted")
	}
	if _, e = VirtualRegistry.Handle(ctx, sid, nil, "upload_image", json.RawMessage(`{"local_path":"/tmp/image.png"}`)); e == nil {
		t.Fatal("disabled image input accepted")
	}
	for i := range models {
		models[i].Enabled = false
	}
	if e = s.Update(1, detail.ID, &dto.AudioGenConfigReq{Models: &models}); e == nil {
		t.Fatal("enabled configuration lost every slot")
	}
	cfg, _ = model.GetAudioGenConfig(1, detail.ID)
	for _, slot := range cfg.Models {
		if slot.Enabled != (slot.Kind != "audio") {
			t.Fatal("failed update was not atomic", slot.Kind)
		}
	}
	if e = s.Delete(1, detail.ID); e != nil {
		t.Fatal(e)
	}
	if VirtualRegistry.IsVirtual(sid) {
		t.Fatal("deleted handler retained")
	}
	var count int64
	model.DB.Model(&model.AudioGenConnection{}).Where("config_id = ?", detail.ID).Count(&count)
	if count != 0 {
		t.Fatal("deleted configuration retained credentials")
	}
}
func TestAudioConnectionsKeepKeysAndRejectInvalidChanges(t *testing.T) {
	audioServiceSetup(t)
	s := &AudioGenService{}
	missingProtocol := audioServiceReq()
	(*missingProtocol.Connections)[0].Protocol = ""
	if _, e := s.Create(1, missingProtocol); e == nil {
		t.Fatal("connection without a protocol was accepted")
	}
	detail, e := s.Create(1, audioServiceReq())
	if e != nil {
		t.Fatal(e)
	}
	connections := []dto.AudioGenConnectionReq{{Key: "main", Protocol: "openrouter", BaseURL: "https://gateway.example/v1", APIKey: igPtr("")}}
	if e = s.Update(1, detail.ID, &dto.AudioGenConfigReq{Connections: &connections}); e != nil {
		t.Fatal(e)
	}
	cfg, _ := model.GetAudioGenConfig(1, detail.ID)
	if cfg.Connections[0].APIKey == "" {
		t.Fatal("blank replaced key")
	}
	connections[0].Protocol = ""
	if e = s.Update(1, detail.ID, &dto.AudioGenConfigReq{Connections: &connections}); e == nil {
		t.Fatal("empty connection protocol silently reused stored protocol")
	}
	connections[0].Protocol = "openrouter"
	connections[0].BaseURL = "https://different.example/v1"
	if e = s.Update(1, detail.ID, &dto.AudioGenConfigReq{Connections: &connections}); e == nil {
		t.Fatal("sent stored key to changed URL")
	}
	models := *audioServiceReq().Models
	models = append(models, models[0])
	if e = s.Update(1, detail.ID, &dto.AudioGenConfigReq{Models: &models}); e == nil {
		t.Fatal("duplicate slot accepted")
	}
	if _, e = s.Models(context.Background(), 2, &dto.AudioGenProbeReq{ConfigID: detail.ID, Kind: "music"}); e == nil {
		t.Fatal("cross-user discovery")
	}
	if _, e = s.Test(context.Background(), 2, &dto.AudioGenProbeReq{ConfigID: detail.ID, Kind: "speech", Arguments: json.RawMessage(`{"text":"hello"}`)}); e == nil {
		t.Fatal("cross-user draft test")
	}
}
