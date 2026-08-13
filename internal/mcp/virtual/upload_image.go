package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
)

// UploadStore is the configured vision-upload blob backend, set by
// router.InitGateway. Held here (rather than reading service.UploadStore)
// because importing service from this package would form an import cycle
// (service → cloud → handler → virtual). Same instance as service.UploadStore.
var UploadStore storage.Storage

// UploadImageToolName is the built-in (non-editable) vision tool that stages a
// local image for vision analysis via a presigned-PUT curl command. Unlike
// analyze_image/describe_scene it is NOT a per-VisionConfig field: it is a
// fixed tool appended to every vision service by buildToolsCache and dispatched
// through the per-user VirtualToolRegistry like the other vision tools.
const UploadImageToolName = "vision.upload_image"

// uploadImageDesc is the fixed description of the built-in upload_image tool.
// It is intentionally a constant (not a per-config field) so the workflow
// guidance stays consistent across every vision service.
const uploadImageDesc = "Stage a LOCAL image file for vision analysis WITHOUT embedding base64. " +
	"Pass local_path; you get back a ready curl command (upload_command) that uploads the file " +
	"straight to storage with NO API key in it, plus a file_url. Workflow: " +
	"1) call this with local_path; 2) run upload_command via your Bash/shell tool; " +
	"3) call vision.analyze_image with image_url=file_url. For SMALL images you may instead " +
	"inline base64 directly to analyze_image; use this tool for larger images."

const uploadLocalPathDesc = "Absolute path of the image on your machine. The server cannot read your disk; it is used only to template the returned curl command. The file must exist locally."

// UploadImageTool returns a FRESH tool definition map for the built-in
// upload_image tool. Each call returns a new map because CollectToolsForGroups
// mutates the "name" entry in place (prepending "<service>__"), so sharing one
// instance across vision services would corrupt it. Name, description and
// schema are fixed constants — not editable per VisionConfig.
func UploadImageTool() map[string]interface{} {
	return map[string]interface{}{
		"name":        UploadImageToolName,
		"description": uploadImageDesc,
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"local_path": map[string]string{
					"type":        "string",
					"description": uploadLocalPathDesc,
				},
			},
			"required": []string{"local_path"},
		},
	}
}

// callerUserIDKey carries the calling user's id (the API key owner) through the
// virtual-tool dispatch path. upload_image needs it for upload ownership and the
// per-user quota guard, but VirtualToolHandler's signature has no userID param
// (changing it would ripple to the camera handler). The gateway injects it into
// ctx at the two virtual dispatch sites; VisionHandler reads it back.
type callerUserIDKey struct{}

// WithCallerUserID returns a copy of ctx carrying the calling user's id.
func WithCallerUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, callerUserIDKey{}, uid)
}

// CallerUserID extracts the calling user's id injected by WithCallerUserID.
// Returns 0 when absent (callers fall back to the vision config owner).
func CallerUserID(ctx context.Context) int64 {
	if v, ok := ctx.Value(callerUserIDKey{}).(int64); ok {
		return v
	}
	return 0
}

// handleUploadImage creates a presigned-PUT upload slot for a local image and
// returns a ready-to-run curl command (no API key) plus the file_url to pass to
// analyze_image. The bytes never enter the model context: the model runs the
// curl via its shell tool, uploading straight to storage (the local PUT endpoint
// or an S3 bucket), then calls analyze_image with file_url. See §14 of the
// vision transport design doc.
func handleUploadImage(ctx context.Context, userID int64, args json.RawMessage) (json.RawMessage, error) {
	if UploadStore == nil {
		return nil, fmt.Errorf("upload storage not initialized")
	}
	var p struct {
		LocalPath string `json:"local_path"`
	}
	_ = json.Unmarshal(args, &p)
	if p.LocalPath == "" {
		return nil, fmt.Errorf("local_path is required")
	}

	// Per-user upload quota (shared guardrail with the multipart path).
	if max := model.GetOptionInt64("MaxUploadsPerUser"); max > 0 {
		if n, err := model.CountUploadsByUser(userID); err == nil && n >= max {
			return nil, fmt.Errorf("upload quota exceeded: %d/%d active uploads; delete old images or wait for cleanup", n, max)
		} else if err != nil {
			return nil, fmt.Errorf("check upload quota: %w", err)
		}
	}

	// UUID key (NOT content-addressed): the server cannot hash bytes it has not
	// seen, so dedup is sacrificed on this path. Images are short-lived (cleaned
	// within retention), so the trade-off is acceptable. Sharded to match the
	// ContentKey shape and pass both backends' key validators.
	id := uuid.NewString()
	key := id[:2] + "/" + id

	// Pending row up front so cleanup tracks the slot even if the model never
	// runs the curl (fast-reaped after the PUT URL expires; see §15.4).
	img := &model.UploadedImage{
		UserID:     userID,
		StorageKey: key,
		Backend:    UploadStore.Backend(),
		Status:     model.UploadStatusPending,
	}
	if err := img.Insert(); err != nil {
		return nil, fmt.Errorf("create upload slot: %w", err)
	}

	putTTL := presignedPutTTL()
	getTTL := signedGetTTL()

	putURL, err := UploadStore.PutURL(ctx, key, putTTL)
	if err != nil {
		_ = model.DeleteUploadedImageByID(img.ID)
		return nil, fmt.Errorf("sign upload url: %w", err)
	}
	fileURL, err := UploadStore.PublicURL(ctx, key, getTTL)
	if err != nil {
		_ = model.DeleteUploadedImageByID(img.ID)
		return nil, fmt.Errorf("sign file url: %w", err)
	}

	cmd := fmt.Sprintf("curl -X PUT -T %q %q", p.LocalPath, putURL)
	nextStep := "Run upload_command via your Bash/shell tool (no API key needed), then call vision.analyze_image with image_url=<file_url>. Do NOT pass image bytes or base64 to any tool."

	// Wrap as a standard MCP tools/call result: {content: [{type:"text", text:...}]}.
	// The text body restates the command + file_url + next_step so a model reading
	// the result sees the ready-to-run instruction (the most effective guidance
	// channel, per §14.8).
	result := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": fmt.Sprintf(
				"Upload slot created. Run this command now, then call vision.analyze_image with image_url.\n\n"+
					"upload_command: %s\nfile_url: %s\nexpires_in: %ds\nkey: %s\n\n%s",
				cmd, fileURL, int(putTTL.Seconds()), key, nextStep)},
		},
	}
	return json.Marshal(result)
}

func presignedPutTTL() time.Duration {
	if s := model.GetOptionInt("PresignedPutTTLSeconds"); s > 0 {
		return time.Duration(s) * time.Second
	}
	return 10 * time.Minute
}

func signedGetTTL() time.Duration {
	if s := model.GetOptionInt("SignedURLTTLSeconds"); s > 0 {
		return time.Duration(s) * time.Second
	}
	return time.Hour
}
