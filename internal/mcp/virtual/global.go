package virtual

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mujkjk/newmcp/internal/storage"
)

// UploadStore is the configured vision-upload blob backend, set by
// router.InitGateway. Held here (rather than reading service.UploadStore)
// because importing service from this package would form an import cycle
// (service → cloud → handler → virtual). Same instance as service.UploadStore.
var UploadStore storage.Storage

// UploadImageToolName is the global (non-per-user) virtual tool that stages a
// local image for vision analysis via a presigned-PUT curl command. It belongs
// to no group/VisionConfig, so it is dispatched via HandleGlobalTool rather than
// the per-user VirtualToolRegistry.
const UploadImageToolName = "vision.upload_image"

// GlobalTools are virtual tools not tied to any per-user service — always
// available alongside vision, dispatched via HandleGlobalTool before the
// per-user registry and group-scope checks. Currently: upload_image.
var GlobalTools = []map[string]interface{}{
	{
		"name": UploadImageToolName,
		"description": "Stage a LOCAL image file for vision analysis WITHOUT embedding base64. " +
			"Pass local_path; you get back a ready curl command (upload_command) that uploads the file " +
			"straight to storage with NO API key in it, plus a file_url. Workflow: " +
			"1) call this with local_path; 2) run upload_command via your Bash/shell tool; " +
			"3) call vision.analyze_image with image_url=file_url. For SMALL images you may instead " +
			"inline base64 directly to analyze_image; use this tool for larger images.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"local_path": map[string]string{
					"type":        "string",
					"description": "Absolute path of the image on your machine. The server cannot read your disk; it is used only to template the returned curl command. The file must exist locally.",
				},
			},
			"required": []string{"local_path"},
		},
	},
}

// IsGlobalTool reports whether toolName is one of the always-on global virtual
// tools, so the gateway can short-circuit dispatch before scope/registry logic.
func IsGlobalTool(toolName string) bool {
	switch toolName {
	case UploadImageToolName:
		return true
	}
	return false
}

// HandleGlobalTool dispatches the global virtual tools. Called from routeAndCall
// (direct/group mode direct-call) and handleExecute (smart mode mcp.execute),
// before the per-user registry and group-scope checks.
func HandleGlobalTool(ctx context.Context, userID int64, toolName string, args json.RawMessage) (json.RawMessage, error) {
	switch toolName {
	case UploadImageToolName:
		return handleUploadImage(ctx, userID, args)
	}
	return nil, fmt.Errorf("unknown global tool: %s", toolName)
}
