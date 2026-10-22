package audiogen

import "fmt"

var ChannelProtocols = []string{"openai", "gemini", "openrouter", "minimax"}

func DefaultChannelURL(channel string) string {
	switch channel {
	case "openai":
		return "https://api.openai.com/v1"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "minimax":
		return "https://api.minimax.io/v1"
	default:
		return ""
	}
}

// ChannelProtocol resolves a connection's protocol to an adapter for its purpose.
func ChannelProtocol(channel, kind string) (string, error) {
	if !Contains([]string{"speech", "audio", "music"}, kind) {
		return "", fmt.Errorf("invalid audio kind %q", kind)
	}
	protocol := ""
	switch channel {
	case "openai":
		if kind == "speech" {
			protocol = "openai_speech"
		} else if kind == "audio" {
			protocol = "openai_chat_audio"
		}
	case "openrouter":
		protocol = "openrouter_chat_audio"
		if kind == "speech" {
			protocol = "openrouter_speech"
		}
	case "gemini":
		if kind != "audio" {
			protocol = "gemini_generate_content"
		}
	case "minimax":
		if kind == "music" {
			protocol = "minimax_music"
		}
	default:
		return "", fmt.Errorf("invalid audio connection protocol %q", channel)
	}
	if protocol == "" {
		return "", fmt.Errorf("connection protocol %s does not support %s", channel, kind)
	}
	return protocol, nil
}
