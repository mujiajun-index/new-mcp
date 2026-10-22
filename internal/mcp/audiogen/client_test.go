package audiogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/mujkjk/newmcp/internal/audioasset"
	"github.com/mujkjk/newmcp/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testSlot(kind, protocol string) model.AudioGenModel {
	return model.AudioGenModel{Kind: kind, Protocol: protocol, Model: "test-model", ConnectionKey: "main", Enabled: true, TimeoutSeconds: 10, DefaultParams: "{}", Capabilities: Preset(kind, protocol)}
}
func jsonArgs(s string) map[string]any {
	var p map[string]any
	_ = json.Unmarshal([]byte(s), &p)
	return p
}
func testWAV() []byte { b, _ := audioasset.WAV([]byte{0, 0, 1, 0, 2, 0, 3, 0}, 24000, 1, 16); return b }

func TestProtocolRequestsAndMultimodalInputs(t *testing.T) {
	for _, protocol := range Protocols {
		if protocol == "openai_realtime" {
			continue
		}
		t.Run(protocol, func(t *testing.T) {
			kind := "speech"
			if protocol == "minimax_music" || protocol == "gemini_generate_content" || protocol == "gemini_interactions" {
				kind = "music"
			}
			if protocol == "openai_chat_audio" || protocol == "openrouter_chat_audio" {
				kind = "audio"
			}
			s := testSlot(kind, protocol)
			c := Client{Connection: Connection{BaseURL: "https://gateway.example/proxy/v1", Slot: s}}
			in := Input{Params: jsonArgs(`{"text":"hello","prompt":"make music"}`)}
			if kind == "audio" {
				in.Audios = []Media{{Data: testWAV(), Format: "wav", MIME: "audio/wav"}}
			}
			if kind == "music" && protocol != "minimax_music" {
				in.Params = map[string]any{}
				in.Images = []Media{{Data: []byte{1, 2, 3}, MIME: "image/png"}}
			}
			endpoint, body, e := c.Build(in)
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(endpoint, "/v1/v1/") {
				t.Fatal(endpoint)
			}
			encoded, _ := json.Marshal(body)
			if kind == "audio" && !bytes.Contains(encoded, []byte(`"input_audio"`)) {
				t.Fatal(string(encoded))
			}
			if len(in.Images) > 0 && !bytes.Contains(encoded, []byte(base64.StdEncoding.EncodeToString(in.Images[0].Data))) {
				t.Fatal(string(encoded))
			}
			if kind == "music" && bytes.Contains(encoded, []byte(`"voice"`)) {
				t.Fatal("music included voice", string(encoded))
			}
			if protocol == "minimax_music" && body["output_format"] != "hex" {
				t.Fatal(body)
			}
		})
	}
	for _, protocol := range []string{"gemini_generate_content", "gemini_interactions", "openrouter_speech"} {
		s := testSlot("speech", protocol)
		s.Model = "gemini-3.8-flash-tts"
		c := Client{Connection: Connection{BaseURL: "https://gateway.example", Slot: s}}
		_, b, e := c.Build(Input{Params: jsonArgs(`{"turns":[{"role":"A","text":"hello"},{"role":"B","text":"hi"}],"speakers":[{"role":"A","voice":"Kore"},{"role":"B","voice":"Puck"}]}`)})
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(b)
		if protocol == "openrouter_speech" && b["response_format"] != "pcm" {
			t.Fatal("multi-speaker Gemini TTS must request PCM", b)
		}
		for _, part := range []string{"Kore", "Puck", "hello", "hi"} {
			if !bytes.Contains(raw, []byte(part)) {
				t.Fatal(string(raw))
			}
		}
	}
}
func TestInputValidationPreventsUnsupportedRequests(t *testing.T) {
	cases := []struct {
		kind, protocol, args string
		valid                bool
	}{
		{"audio", "openai_chat_audio", `{"audios":["data:audio/wav;base64,AAAA"]}`, true},
		{"audio", "openai_chat_audio", `{}`, false},
		{"audio", "openai_realtime", `{"prompt":"hello","audios":["url"]}`, false},
		{"audio", "openai_realtime", `{"prompt":"hello","format":"mp3"}`, false},
		{"speech", "openai_speech", `{"text":"hello","extra_body":{"model":"override"}}`, false},
		{"speech", "openai_speech", `{"text":"hello","turns":[]}`, false},
		{"speech", "gemini_generate_content", `{"turns":[{"role":"A","text":"hello","unknown":1}],"speakers":[{"role":"A","voice":"Kore"}]}`, false},
		{"music", "gemini_generate_content", `{"images":["url"]}`, true},
		{"music", "minimax_music", `{"images":["url"]}`, false},
		{"music", "minimax_music", `{"prompt":"jazz"}`, true},
		{"music", "minimax_music", `{"prompt":"jazz","lyrics_optimizer":false}`, false},
		{"music", "minimax_music", `{"lyrics":"[Verse] Hello"}`, true},
		{"music", "minimax_music", `{"prompt":"jazz","lyrics":"words","is_instrumental":true}`, false},
		{"music", "minimax_music", `{"prompt":"jazz","is_instrumental":true}`, true},
		{"music", "minimax_music", `{"prompt":"jazz","voice":"alloy"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.protocol+tc.args, func(t *testing.T) {
			s := testSlot(tc.kind, tc.protocol)
			_, e := Normalize(&s, jsonArgs(tc.args), false)
			if (e == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, e)
			}
		})
	}
	unknown := ModelPreset("audio", "openrouter_chat_audio", "custom-model")
	if Contains(unknown.InputModalities, "audio") {
		t.Fatal("unknown model advertised audio without metadata")
	}
}
func TestHTTPAudioResponsesAndBusinessErrors(t *testing.T) {
	wav := testWAV()
	encoded := base64.StdEncoding.EncodeToString(wav)
	cases := []struct {
		protocol, kind, response string
		bad                      bool
	}{
		{"openai_chat_audio", "audio", `{"choices":[{"message":{"audio":{"data":"` + encoded + `","transcript":"spoken answer"}}}],"usage":{"total_tokens":1}}`, false},
		{"minimax_music", "music", `{"base_resp":{"status_code":0},"data":{"status":2,"audio":"` + hex.EncodeToString(wav) + `"}}`, false},
		{"minimax_music", "music", `{"base_resp":{"status_code":1004,"status_msg":"bad key"},"data":{}}`, true},
		{"gemini_generate_content", "music", `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/wav","data":"` + encoded + `"}},{"text":"lyrics after audio"}]}}]}`, false},
		{"gemini_interactions", "music", `{"steps":[{"type":"model_output","content":[{"type":"audio","mime_type":"audio/wav","data":"` + encoded + `"},{"type":"text","text":"lyrics after audio"}]}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.protocol+fmt.Sprint(tc.bad), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(tc.protocol, "gemini_") {
					if r.Header.Get("x-goog-api-key") != "secret" {
						t.Error("missing Gemini auth")
					}
				} else if r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("missing auth")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.response)
			}))
			defer server.Close()
			s := testSlot(tc.kind, tc.protocol)
			out, e := (&Client{Connection: Connection{BaseURL: server.URL, APIKey: "secret", Slot: s}}).Generate(context.Background(), Input{Params: map[string]any{"prompt": "hello"}})
			if tc.bad {
				if e == nil {
					t.Fatal("accepted business failure")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if len(out.Audios) != 1 || !bytes.Equal(out.Audios[0].Data, wav) {
				t.Fatal("missing audio")
			}
			if strings.HasPrefix(tc.protocol, "gemini_") && out.Lyrics != "lyrics after audio" {
				t.Fatal(out.Lyrics)
			}
		})
	}
}
func TestOpenRouterStreamRequiresCompletion(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(testWAV())
	complete := `data: {"choices":[{"delta":{"audio":{"data":"` + encoded + `","transcript":"answer"}}}]}` + "\n\n" + `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
	c := Client{Connection: Connection{Slot: testSlot("audio", "openrouter_chat_audio")}}
	out, e := c.stream(strings.NewReader(complete))
	if e != nil || out.Transcript != "answer" {
		t.Fatal(out, e)
	}
	_, e = c.stream(strings.NewReader(strings.Split(complete, "data: {\"choices\":[{\"delta\":{}")[0]))
	if e == nil || !IsUncertain(e) {
		t.Fatalf("truncated stream should be uncertain: %v", e)
	}
}
func TestRealtimeCollectsDeltasAndChecksFinalStatus(t *testing.T) {
	for _, status := range []string{"completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			upgrader := websocket.Upgrader{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, e := upgrader.Upgrade(w, r, nil)
				if e != nil {
					return
				}
				defer ws.Close()
				var event map[string]any
				if ws.ReadJSON(&event) != nil {
					return
				}
				if event["type"] != "session.update" {
					t.Error(event)
				}
				_ = ws.WriteJSON(map[string]any{"type": "session.updated"})
				for i := 0; i < 2; i++ {
					if ws.ReadJSON(&event) != nil {
						return
					}
				}
				_ = ws.WriteJSON(map[string]any{"type": "response.output_audio.delta", "delta": base64.StdEncoding.EncodeToString([]byte{0, 0, 1, 0})})
				_ = ws.WriteJSON(map[string]any{"type": "response.output_audio_transcript.delta", "delta": "hello"})
				_ = ws.WriteJSON(map[string]any{"type": "response.done", "response": map[string]any{"status": status}})
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			c := Client{Connection: Connection{BaseURL: server.URL, APIKey: "secret", Slot: testSlot("audio", "openai_realtime")}}
			out, e := c.Generate(ctx, Input{Params: map[string]any{"prompt": "hello"}})
			if status == "failed" {
				if e == nil {
					t.Fatal("accepted incomplete response")
				}
				return
			}
			if e != nil || out.Transcript != "hello" || out.Audios[0].Rate != 24000 {
				t.Fatal(out, e)
			}
		})
	}
}
