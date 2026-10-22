package audiogen

import "testing"

func TestChannelProtocolByPurpose(t *testing.T) {
	rows := []struct {
		channel string
		want    [3]string
	}{
		{"openai", [3]string{"openai_speech", "openai_chat_audio", ""}},
		{"openrouter", [3]string{"openrouter_speech", "openrouter_chat_audio", "openrouter_chat_audio"}},
		{"gemini", [3]string{"gemini_generate_content", "", "gemini_generate_content"}},
		{"minimax", [3]string{"", "", "minimax_music"}},
	}
	for _, row := range rows {
		for i, kind := range []string{"speech", "audio", "music"} {
			got, err := ChannelProtocol(row.channel, kind)
			if got != row.want[i] || (err != nil) != (row.want[i] == "") {
				t.Fatalf("%s/%s: got %q, %v; want %q", row.channel, kind, got, err, row.want[i])
			}
		}
	}
	for _, channel := range []string{"", "other", "openai_realtime", "gemini_interactions"} {
		if _, err := ChannelProtocol(channel, "speech"); err == nil {
			t.Fatal("missing or unknown channel was accepted")
		}
	}
	if _, err := ChannelProtocol("openrouter", "other"); err == nil {
		t.Fatal("unknown purpose was accepted")
	}
}
