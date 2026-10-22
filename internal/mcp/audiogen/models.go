package audiogen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mujkjk/newmcp/internal/audioasset"
)

type ModelInfo struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	SupportedVoices  []string `json:"supported_voices,omitempty"`
}

func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if c.Connection.Slot.Protocol == "minimax_music" {
		return []ModelInfo{{ID: "music-3.0", Name: "MiniMax Music 3", InputModalities: []string{"text"}, OutputModalities: []string{"audio"}}}, nil
	}
	base, e := BaseURL(c.Connection.BaseURL, c.Connection.Slot.Protocol)
	if e != nil {
		return nil, e
	}
	modelsURL := base + "/models"
	outputModality := ""
	switch c.Connection.Slot.Protocol {
	case "openrouter_speech":
		outputModality = "speech"
	case "openrouter_chat_audio":
		outputModality = "audio"
	}
	if outputModality != "" {
		modelsURL += "?output_modalities=" + outputModality
	}
	req, e := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if e != nil {
		return nil, e
	}
	if strings.HasPrefix(c.Connection.Slot.Protocol, "gemini_") {
		req.Header.Set("x-goog-api-key", c.Connection.APIKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.Connection.APIKey)
	}
	resp, e := c.httpClient().Do(req)
	if e != nil {
		return nil, fmt.Errorf("could not query models; custom model IDs can be entered manually")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("model discovery unavailable (HTTP %d); enter a model ID manually", resp.StatusCode)
	}
	b, e := audioasset.Read(resp.Body, 16<<20)
	if e != nil {
		return nil, e
	}
	var v struct {
		Data []struct {
			ID              string   `json:"id"`
			Name            string   `json:"name"`
			SupportedVoices []string `json:"supported_voices"`
			Architecture    struct {
				Inputs  []string `json:"input_modalities"`
				Outputs []string `json:"output_modalities"`
			} `json:"architecture"`
		} `json:"data"`
		Models []struct {
			Name             string   `json:"name"`
			DisplayName      string   `json:"displayName"`
			SupportedActions []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil, fmt.Errorf("invalid models response")
	}
	out := []ModelInfo{}
	for _, m := range v.Data {
		if len(m.Architecture.Outputs) > 0 && !Contains(m.Architecture.Outputs, "audio") && !Contains(m.Architecture.Outputs, "speech") {
			continue
		}
		if outputModality != "" && len(m.Architecture.Outputs) > 0 && !Contains(m.Architecture.Outputs, outputModality) {
			continue
		}
		for i, v := range m.Architecture.Outputs {
			if v == "speech" {
				m.Architecture.Outputs[i] = "audio"
			}
		}
		out = append(out, ModelInfo{ID: m.ID, Name: m.Name, InputModalities: m.Architecture.Inputs, OutputModalities: m.Architecture.Outputs, SupportedVoices: m.SupportedVoices})
	}
	for _, m := range v.Models {
		out = append(out, ModelInfo{ID: strings.TrimPrefix(m.Name, "models/"), Name: m.DisplayName})
	}
	return out, nil
}
