package audiogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mujkjk/newmcp/internal/taskreference"
	"github.com/mujkjk/newmcp/model"
)

func TestImageOnlyMusicSnapshotsAndContentDedupe(t *testing.T) {
	db, store := managerDB(t)
	var picture bytes.Buffer
	if e := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); e != nil {
		t.Fatal(e)
	}
	encodedImage := base64.StdEncoding.EncodeToString(picture.Bytes())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		b, _ := json.Marshal(body)
		if !bytes.Contains(b, []byte(encodedImage)) || bytes.Contains(b, []byte(`"voice"`)) {
			t.Error("invalid image-only music request")
		}
		fmt.Fprintf(w, `{"candidates":[{"content":{"parts":[{"text":"song lyrics"},{"inlineData":{"mimeType":"audio/wav","data":%q}}]}}]}`, base64.StdEncoding.EncodeToString(testWAV()))
	}))
	defer server.Close()
	cfg := managerConfig(t, server.URL, "music")
	cfg.Models = []model.AudioGenModel{testSlot("music", "gemini_generate_content")}
	m := NewManager(db, store)
	args := map[string]any{"images": []any{"data:image/png;base64," + encodedImage}, "request_id": "same-image"}
	task, e := m.Submit(context.Background(), 1, cfg, "music", args, "test")
	if e != nil {
		t.Fatal(e)
	}
	args["images"] = []any{encodedImage}
	same, e := m.Submit(context.Background(), 1, cfg, "music", args, "test")
	if e != nil || same.ID != task.ID {
		t.Fatal("changed reference transport changed content fingerprint", e)
	}
	m.Start(context.Background())
	defer m.Close()
	done := awaitAudio(t, m, task.ID, "completed", "ready")
	if done.Lyrics != "song lyrics" {
		t.Fatal(done)
	}
	view := m.View(done)
	refs := view["reference_images"].([]taskreference.View)
	if len(refs) != 1 || refs[0].Status != "ready" {
		t.Fatal(refs)
	}
	raw, _ := json.Marshal(view)
	if bytes.Contains(raw, []byte(encodedImage)) {
		t.Fatal("history exposed inline input")
	}
}

func TestGeminiSingleSpeakerUsesVoiceArrayAndStyleMetadata(t *testing.T) {
	for _, protocol := range []string{"gemini_interactions", "gemini_generate_content"} {
		s := testSlot("speech", protocol)
		s.Model = "gemini-3.8-flash-tts"
		c := Client{Connection: Connection{BaseURL: "https://example.com", Slot: s}}
		_, body, e := c.Build(Input{Params: map[string]any{"text": "hello", "instructions": "warm", "voice": "Kore"}})
		if e != nil {
			t.Fatal(e)
		}
		if protocol == "gemini_interactions" {
			config := body["generation_config"].(map[string]any)
			if _, ok := config["speech_config"].([]any); !ok {
				t.Fatal("single-speaker speech_config must be an array", config)
			}
		}
		raw, _ := json.Marshal(body)
		if !bytes.Contains(raw, []byte(`"text":"hello"`)) || !bytes.Contains(raw, []byte(`"style":"warm"`)) {
			t.Fatal("delivery instructions were mixed into spoken text", string(raw))
		}
	}
}

func TestSpeechModelDiscoveryAcceptsSpeechModality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.URL.Query().Get("output_modalities") != "speech" {
			t.Errorf("speech catalog requires explicit output_modalities=speech: %s", r.URL)
		}
		fmt.Fprint(w, `{"data":[{"id":"google/gemini-3.8-flash-tts","architecture":{"input_modalities":["text"],"output_modalities":["speech"]}},{"id":"chat-only","architecture":{"input_modalities":["text"],"output_modalities":["text"]}}]}`)
	}))
	defer server.Close()
	c := Client{Connection: Connection{BaseURL: server.URL, Slot: testSlot("speech", "openrouter_speech")}}
	models, e := c.ListModels(context.Background())
	if e != nil || len(models) != 1 || !Contains(models[0].OutputModalities, "audio") {
		t.Fatal(models, e)
	}
}

func TestOpenRouterModelDiscoverySeparatesSpeechAndChatAudio(t *testing.T) {
	for _, tc := range []struct {
		protocol string
		modality string
		model    string
	}{
		{"openrouter_speech", "speech", "speech-model"},
		{"openrouter_chat_audio", "audio", "audio-model"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/gateway/v1/models" || r.URL.Query().Get("output_modalities") != tc.modality {
					t.Errorf("wrong catalog request: %s", r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer catalog-key" {
					t.Error("missing configured connection credentials")
				}
				// Some gateways return an unfiltered catalog; incompatible protocols
				// still must not be offered as selectable models.
				fmt.Fprint(w, `{"data":[{"id":"speech-model","architecture":{"input_modalities":["text"],"output_modalities":["speech"]}},{"id":"audio-model","architecture":{"input_modalities":["text","audio"],"output_modalities":["text","audio"]}},{"id":"chat-only","architecture":{"output_modalities":["text"]}},{"id":"custom-alias"}]}`)
			}))
			defer server.Close()
			c := Client{Connection: Connection{BaseURL: server.URL + "/gateway/v1/", APIKey: "catalog-key", Slot: testSlot("speech", tc.protocol)}}
			models, e := c.ListModels(context.Background())
			if e != nil || len(models) != 2 || models[0].ID != tc.model || models[1].ID != "custom-alias" || !Contains(models[0].OutputModalities, "audio") {
				t.Fatal(models, e)
			}
		})
	}
}

func TestModelDiscoveryPreservesProviderVoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"microsoft/mai-voice-2.1","architecture":{"output_modalities":["speech"]},"supported_voices":["en-US-Harper:MAI-Voice-2.1","zh-CN-Xiaochen:MAI-Voice-2.1"]},{"id":"custom-alias","supported_voices":null}]}`)
	}))
	defer server.Close()
	c := Client{Connection: Connection{BaseURL: server.URL, Slot: testSlot("speech", "openrouter_speech")}}
	models, err := c.ListModels(context.Background())
	if err != nil || len(models) != 2 {
		t.Fatal(models, err)
	}
	if len(models[0].SupportedVoices) != 2 || models[0].SupportedVoices[0] != "en-US-Harper:MAI-Voice-2.1" || len(models[1].SupportedVoices) != 0 {
		t.Fatal("provider voices changed or leaked between models", models)
	}
	raw, err := json.Marshal(models)
	if err != nil || !bytes.Contains(raw, []byte(`"supported_voices":["en-US-Harper:MAI-Voice-2.1","zh-CN-Xiaochen:MAI-Voice-2.1"]`)) {
		t.Fatal("voice metadata missing from discovery response", string(raw), err)
	}
}
