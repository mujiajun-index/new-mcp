package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	case strings.HasSuffix(toolName, "capture") || toolName == "camera.capture":
		return handleCapture(cam.ID)
	case strings.HasSuffix(toolName, "analyze") || toolName == "camera.analyze":
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
		return nil, fmt.Errorf("摄像头未开启或无可用画面，请先在前端开启摄像头")
	}

	b64 := EncodeFrameToBase64(frame)
	resp := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "image", "data": b64, "mimeType": "image/jpeg"},
			{"type": "text", "text": fmt.Sprintf("截取时间: %s", capturedAt.Format("2006-01-02 15:04:05"))},
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
		return nil, fmt.Errorf("摄像头未开启或无可用画面，请先在前端开启摄像头")
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
		EndpointURL: vc.EndpointURL,
		ApiKey:      vc.ApiKey,
		ModelName:   vc.ModelName,
		MaxTokens:   vc.MaxTokens,
	}

	systemPrompt := vc.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a precise image analysis assistant. Examine the provided camera frame and identify the objects, people, text, and activity it contains. Be accurate, objective, and thorough."
	}
	userPrompt := "Analyze this camera frame in detail. Identify and describe every person and object, note any visible text or signage, and summarize the current activity and setting."
	if params.Prompt != "" {
		userPrompt = params.Prompt
	}

	b64 := EncodeFrameToBase64(frame)
	result, err := client.Analyze(ctx, systemPrompt, userPrompt, b64, "image/jpeg")
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
