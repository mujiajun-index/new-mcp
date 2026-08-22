package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/camera"
	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/model"
)

var StreamManager *camera.CameraStreamManager

func CameraHandler(ctx context.Context, serviceID int64, config map[string]interface{}, toolName string, args json.RawMessage) (json.RawMessage, error) {
	refID, _ := config["ref_id"].(float64)
	if refID == 0 {
		return nil, fmt.Errorf("invalid ref_id in virtual service config")
	}

	cam, err := model.GetCameraByServiceID(serviceID)
	if err != nil {
		return nil, fmt.Errorf("camera not found: %w", err)
	}

	switch {
	case strings.HasSuffix(toolName, "capture"):
		return handleCapture(cam.ID)
	case strings.HasSuffix(toolName, "analyze"):
		return handleAnalyze(ctx, cam, args)
	default:
		return nil, fmt.Errorf("unknown camera tool: %s", toolName)
	}
}

func handleCapture(cameraID int64) (json.RawMessage, error) {
	if StreamManager == nil {
		return nil, fmt.Errorf("camera stream manager not initialized")
	}

	frame, capturedAt, ok := StreamManager.GetLatestFrame(cameraID)
	if !ok {
		return nil, fmt.Errorf("camera is off or no frame is available; please enable the camera in the frontend first")
	}

	b64 := EncodeFrameToBase64(frame)
	resp := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "image", "data": b64, "mimeType": "image/jpeg"},
			{"type": "text", "text": fmt.Sprintf("Captured at: %s", capturedAt.Format("2006-01-02 15:04:05"))},
		},
	}
	return json.Marshal(resp)
}

func handleAnalyze(ctx context.Context, cam *model.Camera, args json.RawMessage) (json.RawMessage, error) {
	if StreamManager == nil {
		return nil, fmt.Errorf("camera stream manager not initialized")
	}

	frame, _, ok := StreamManager.GetLatestFrame(cam.ID)
	if !ok {
		return nil, fmt.Errorf("camera is off or no frame is available; please enable the camera in the frontend first")
	}

	if cam.VisionConfigID == nil {
		return nil, fmt.Errorf("camera has no vision config bound")
	}

	var vc model.VisionConfig
	if err := model.DB.First(&vc, *cam.VisionConfigID).Error; err != nil {
		return nil, fmt.Errorf("vision config not found: %w", err)
	}

	var params struct {
		Prompt string `json:"prompt"`
	}
	_ = json.Unmarshal(args, &params)

	client := &vision.VisionClient{
		Provider:       vc.Provider,
		EndpointURL:    vc.EndpointURL,
		ApiKey:         vc.ApiKey,
		ModelName:      vc.ModelName,
		MaxTokens:      vc.MaxTokens,
		AnalyzeTimeout: time.Duration(vc.AnalyzeTimeoutSeconds) * time.Second,
	}

	// Same prompt resolution as analyze_image: the bound VisionConfig's
	// SystemPrompt, falling back to the shared analyze defaults when empty or
	// when the caller passes no prompt — so a camera frame and an uploaded
	// image get identical treatment from the same config.
	systemPrompt := vc.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultAnalyzeSystemPrompt
	}
	userPrompt := defaultAnalyzeUserPrompt
	if params.Prompt != "" {
		userPrompt = params.Prompt
	}

	// Apply the same optional read-path transforms (resize / re-encode) as the
	// vision path, right before the upstream call. Camera frames are usually
	// small JPEGs, so this is typically a no-op via the fast path; including it
	// keeps both byte sources uniform. Toggles default off.
	imgIn := vision.Transform(vision.ImageInput{Bytes: frame, MediaType: "image/jpeg"}, loadTransformOpts())
	result, err := client.Analyze(ctx, systemPrompt, userPrompt, imgIn)
	if err != nil {
		return nil, err
	}

	resp := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": result},
		},
	}
	return json.Marshal(resp)
}
