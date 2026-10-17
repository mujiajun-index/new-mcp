package imagegentask

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/mujkjk/newmcp/model"
)

func (m *Manager) MCPResult(ctx context.Context, t *model.ImageGenTask, mode string) (json.RawMessage, error) {
	views, err := Views([]model.ImageGenTask{*t})
	if err != nil {
		return nil, err
	}
	view := views[0]
	content := []map[string]any{}
	inline := []map[string]any{}
	if mode == "" {
		mode = "url"
	}
	if mode != "url" && mode != "inline" {
		return nil, fmt.Errorf("return_mode must be url or inline")
	}
	limit := model.GetOptionInt64("ImageGenInlineMaxBytes")
	if limit <= 0 {
		limit = 1 << 20
	}
	var inlineBytes int64
	for index, img := range view.Images {
		if img.URL == "" {
			continue
		}
		content = append(content, map[string]any{"type": "resource_link", "uri": img.URL, "name": fmt.Sprintf("generated-image-%d", index+1), "mimeType": img.MIME, "size": img.Size})
		if mode == "inline" {
			inlineBytes += (img.Size + 2) / 3 * 4
			if inlineBytes <= limit {
				image, err := m.loadReference(ctx, t.UserID, img.UploadID)
				if err != nil {
					continue
				}
				inline = append(inline, map[string]any{"type": "image", "data": base64.StdEncoding.EncodeToString(image.Bytes), "mimeType": image.MIME})
			}
		}
	}
	if mode == "inline" && inlineBytes > limit {
		inline = nil
		view.Warnings = append(view.Warnings, fmt.Sprintf("Inline response exceeds %d bytes; use the image links.", limit))
	}
	makeResult := func() (json.RawMessage, error) {
		data, _ := json.Marshal(view)
		parts := append([]map[string]any{{"type": "text", "text": string(data)}}, content...)
		parts = append(parts, inline...)
		return json.Marshal(map[string]any{"content": parts, "structuredContent": view, "isError": !Active(t) && t.Status != "completed"})
	}
	result, err := makeResult()
	if len(inline) > 0 && int64(len(result)) > limit {
		inline = nil
		view.Warnings = append(view.Warnings, fmt.Sprintf("Inline response exceeds %d bytes; use the image links.", limit))
		return makeResult()
	}
	return result, err
}
