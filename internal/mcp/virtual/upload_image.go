package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
// analyze_image it is NOT a per-VisionConfig field: it is a
// fixed tool appended to every vision service by buildToolsCache and dispatched
// through the per-user VirtualToolRegistry like the other vision tools.
const UploadImageToolName = "upload_image"

// uploadImageDesc is the fixed description of the built-in upload_image tool.
// It is intentionally a constant (not a per-config field) so the workflow
// guidance stays consistent across every vision service.
const uploadImageDesc = "Stage a LOCAL image file for vision analysis. Workflow: 1) call this with local_path; " +
	"2) run the returned upload_command via your Bash/shell tool; 3) call analyze_image with the returned image_url. " +
	"For SMALL images you may instead inline base64 directly to analyze_image; use this tool for larger images."

const uploadLocalPathDesc = "Absolute path of the image on your machine, in your system's native form " +
	"(Windows: C:\\Users\\me\\photo.jpg; macOS/Linux: /Users/me/photo.jpg). The server cannot read your " +
	"disk; the path is used to detect your OS and to template the returned curl command. The file must exist locally."

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
// returns a ready-to-run curl command (no API key) plus the image_url to pass to
// analyze_image. The bytes never enter the model context: the model runs the
// curl via its shell tool, uploading straight to storage (the local PUT endpoint
// or an S3 bucket), then calls analyze_image with the image_url. See §14 of the
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
	if err := img.InsertWithGeneratedShortID(); err != nil {
		return nil, fmt.Errorf("create upload slot: %w", err)
	}

	putTTL := presignedPutTTL()

	// Short capability URLs: /u/<sid>?s=<method-bound MAC>. The PUT (upload_command)
	// and GET (image_url) URLs share the row's short_id and differ only in the
	// method-bound signature, so the model's curl carries no API key and a GET
	// token can't be replayed as a PUT. These are infallible (no presign round-trip,
	// no backend dependency), so the pending slot needs no delete-on-failure cleanup.
	putURL := storage.ShortURL("PUT", img.ShortID)
	imageURL := storage.ShortURL("GET", img.ShortID)

	// Return the ONE curl command matched to the caller's OS, inferred from
	// local_path (drive letter / backslash → Windows → curl.exe; otherwise →
	// curl). Detection is reliable for the absolute paths agents pass; a
	// relative path with no signal defaults to the unix `curl` form (also valid
	// in Git Bash and cmd.exe). We deliberately do NOT also emit the other-OS
	// variant: the two differ only in the binary name, so a second line would
	// just duplicate the long presigned URL, and the model self-corrects
	// curl↔curl.exe on the rare wrong-detection case (the same one-shot retry it
	// did before this tool existed).
	cmd := uploadCommand(p.LocalPath, putURL)

	// Standard MCP tools/call result: {content:[{type:"text",text:...}]}. The
	// text is the most effective guidance channel (§14.8): a ready-to-run
	// command + image_url, nothing fixed or duplicated. The returned field is
	// deliberately named image_url — the exact name of analyze_image's
	// parameter — so the model passes it through verbatim with no rename
	// mapping to fumble.
	result := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": fmt.Sprintf(
				"Upload slot ready. Run upload_command, then call analyze_image with the image_url below.\n\n"+
					"upload_command: %s\nimage_url: %s\nexpires_in: %ds",
				cmd, imageURL, int(putTTL.Seconds()))},
		},
	}
	return json.Marshal(result)
}

// looksLikeWindowsPath reports whether local_path is a Windows path (drive-letter
// prefix or any backslash). The server cannot ask the caller's OS, but the path
// the model passes reveals it with near-total reliability: Windows absolute paths
// look like C:\Users\... and unix paths never carry a drive letter or (in
// practice) a backslash. A relative path with no signal is treated as unix — on
// Windows the unix `curl` form still works in Git Bash and cmd.exe, and the
// Windows (curl.exe) variant is always emitted alongside as the fallback.
func looksLikeWindowsPath(p string) bool {
	if strings.Contains(p, `\`) {
		return true
	}
	if len(p) >= 2 && p[1] == ':' {
		c := p[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return true
		}
	}
	return false
}

// quoteArg wraps a shell argument in single quotes so spaces and shell
// metacharacters (&, ?, =, $, ...) survive. Single quotes are literal in
// PowerShell, bash and zsh. Windows filenames cannot contain a single quote, so
// no escaping is needed there; unix paths with an embedded ' are vanishingly
// rare and left unsupported rather than picking one of the divergent escape
// conventions (PowerShell doubles it, bash needs '\”). The path's URL arg is
// HMAC/presign output and never contains a quote either.
func quoteArg(s string) string {
	return "'" + s + "'"
}

// buildUploadCommands returns the curl upload command in Windows and unix form.
// They differ only in the binary name: curl.exe on Windows (PowerShell aliases
// bare `curl` to Invoke-WebRequest, which rejects -T/-X), curl everywhere else.
// Both PUT the file as the request body to the same presigned URL.
func buildUploadCommands(localPath, putURL string) (windowsCmd, unixCmd string) {
	windowsCmd = fmt.Sprintf("curl.exe -X PUT -T %s %s", quoteArg(localPath), quoteArg(putURL))
	unixCmd = fmt.Sprintf("curl -X PUT -T %s %s", quoteArg(localPath), quoteArg(putURL))
	return windowsCmd, unixCmd
}

// uploadCommand returns the single curl upload command matched to the caller's
// OS, inferred from local_path: a Windows path (drive letter / backslash) →
// curl.exe (PowerShell aliases bare `curl` to Invoke-WebRequest); anything else
// → curl (real curl on macOS, Linux, Git Bash, cmd.exe). Exactly one command is
// returned — detection picks it — rather than both OS forms, which differ only
// in the binary name and would needlessly duplicate the long presigned URL.
func uploadCommand(localPath, putURL string) string {
	win, unix := buildUploadCommands(localPath, putURL)
	if looksLikeWindowsPath(localPath) {
		return win
	}
	return unix
}

func presignedPutTTL() time.Duration {
	if s := model.GetOptionInt("PresignedPutTTLSeconds"); s > 0 {
		return time.Duration(s) * time.Second
	}
	return 10 * time.Minute
}
