package audiogen

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mujkjk/newmcp/internal/audioasset"
	"github.com/mujkjk/newmcp/model"
)

func TestGeminiTTSSpeechFormat(t *testing.T) {
	for _, tc := range []struct {
		protocol, model, format, upstream string
		invalid                           bool
	}{
		{"openrouter_speech", "google/gemini-2.5-flash-preview-tts", "", "pcm", false},
		{"openrouter_speech", "google/gemini-3.8-flash-tts", "", "pcm", false},
		{"openrouter_speech", "google/gemini-3.8-flash-tts", "pcm", "pcm", false},
		{"openrouter_speech", "google/gemini-3.8-flash-tts", "wav", "pcm", false},
		{"openrouter_speech", "google/gemini-3.8-flash-tts", "mp3", "", true},
		{"openai_speech", "gpt-4o-mini-tts", "", "mp3", false},
		{"openrouter_speech", "openai/gpt-4o-mini-tts", "", "mp3", false},
		{"openrouter_speech", "custom-alias", "pcm", "pcm", false},
	} {
		t.Run(tc.protocol+"/"+tc.model+"/"+tc.format, func(t *testing.T) {
			s := testSlot("speech", tc.protocol)
			s.Model = tc.model
			p := map[string]any{"text": "hello"}
			if tc.format != "" {
				p["format"] = tc.format
			}
			_, err := Normalize(&s, p, false)
			if (err != nil) != tc.invalid {
				t.Fatalf("unexpected format validation: %v", err)
			}
			c := Client{Connection: Connection{BaseURL: "https://gateway.example/v1", Slot: s}}
			_, body, err := c.Build(Input{Params: p})
			if tc.invalid {
				if err == nil {
					t.Fatal("unsupported Gemini format was sent upstream")
				}
				// Defaults receive the same validation as a test or MCP call.
				s.DefaultParams = `{"format":"mp3"}`
				if ValidateSlot(&s) == nil {
					t.Fatal("accepted an incompatible default format")
				}
				return
			}
			if err != nil || body["response_format"] != tc.upstream {
				t.Fatal(body, err)
			}
		})
	}
}

func TestOpenRouterGeminiPCMMetadata(t *testing.T) {
	for _, tc := range []struct {
		contentType    string
		rate, channels int
	}{
		{"audio/pcm", 24000, 1},
		{"application/octet-stream", 24000, 1},
		{"audio/pcm;rate=16000;channels=2", 16000, 2},
	} {
		t.Run(tc.contentType, func(t *testing.T) {
			pcm := testWAV()[44:]
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["response_format"] != "pcm" {
					t.Error("Gemini TTS must request PCM", body)
				}
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write(pcm)
			}))
			defer server.Close()
			s := testSlot("speech", "openrouter_speech")
			s.Model = "google/gemini-3.8-flash-tts"
			c := Client{Connection: Connection{BaseURL: server.URL, Slot: s}}
			out, err := c.Generate(context.Background(), Input{Params: map[string]any{"text": "hello"}})
			if err != nil {
				t.Fatal(err)
			}
			a := out.Audios[0]
			if a.MIME != "audio/pcm" || a.Rate != tc.rate || a.Channels != tc.channels || a.Bits != 16 || !bytes.Equal(a.Data, pcm) {
				t.Fatal("incorrect PCM metadata", a)
			}
		})
	}
}

func TestOpenRouterGeminiTaskSavesPlayableWAV(t *testing.T) {
	db, store := managerDB(t)
	var calls atomic.Int32
	wav := testWAV()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["response_format"] != "pcm" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Gemini TTS only supports response_format=pcm"}}`))
			return
		}
		w.Header().Set("Content-Type", "audio/pcm")
		_, _ = w.Write(wav[44:])
	}))
	defer server.Close()
	cfg := managerConfig(t, server.URL, "speech")
	cfg.Models[0] = testSlot("speech", "openrouter_speech")
	cfg.Models[0].Model = "google/gemini-3.8-flash-tts"
	m := NewManager(db, store)
	if _, err := m.Submit(context.Background(), 1, cfg, "speech", map[string]any{"text": "hello", "format": "mp3"}, "test"); err == nil {
		t.Fatal("incompatible output format should fail before task submission")
	}
	task, err := m.Submit(context.Background(), 1, cfg, "speech", map[string]any{"text": "hello"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	m.Start(context.Background())
	defer m.Close()
	awaitAudio(t, m, task.ID, "completed", "ready")
	var asset model.AudioAsset
	if err = db.Where("task_id = ? AND source = ?", task.ID, "generated").First(&asset).Error; err != nil {
		t.Fatal(err)
	}
	r, err := store.Get(context.Background(), asset.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	saved, err := audioasset.Read(r, 1024)
	if err != nil || asset.MIME != "audio/wav" || !bytes.Equal(saved, wav) || calls.Load() != 1 {
		t.Fatalf("saved audio must be WAV without regenerating: asset=%+v calls=%d err=%v", asset, calls.Load(), err)
	}
}
