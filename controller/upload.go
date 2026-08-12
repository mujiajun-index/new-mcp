package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
)

var uploadService = &service.UploadService{}

// maxBodySlack is the extra bytes allowed on the multipart request body beyond
// the image cap, to absorb boundary/header framing without rejecting valid
// uploads at the edge. The image itself is bounded again inside UploadService.
const maxBodySlack = 1 << 20 // 1 MiB

// UploadVisionImage accepts a single image under multipart field "image" and
// returns a short-lived signed URL the caller can pass to a vision tool's
// image_url parameter. It is mounted on TWO routes:
//
//	POST /api/v1/vision/upload      (UserAuth / JWT — for the web UI)
//	POST /api/v1/vision/mcp-upload  (APIKeyAuth / sk- — for MCP clients)
//
// gin's radix router forbids mounting two middleware sets on the same path, so
// the MCP path is separate but shares this handler. The two middlewares stash
// the caller's user id under different context keys (user_id vs
// api_key_user_id); either is accepted here.
func UploadVisionImage(c *gin.Context) {
	if service.UploadStore == nil {
		common.Error(c, http.StatusServiceUnavailable, "upload storage not initialized")
		return
	}

	userID := c.GetInt64("user_id")
	if userID == 0 {
		userID = c.GetInt64("api_key_user_id")
	}
	if userID == 0 {
		common.Error(c, http.StatusUnauthorized, "missing user identity")
		return
	}

	maxBytes := model.GetOptionInt64("VisionUploadMaxBytes")
	if maxBytes <= 0 {
		maxBytes = 10 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+maxBodySlack)

	file, _, err := c.Request.FormFile("image")
	if err != nil {
		common.Error(c, http.StatusBadRequest, "invalid upload: "+err.Error())
		return
	}
	defer file.Close()

	res, err := uploadService.Upload(c.Request.Context(), userID, file, maxBytes)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, res)
}

// GetVisionFile serves a previously uploaded vision image by its content key.
// It is mounted on the PUBLIC route group (no UserAuth/APIKeyAuth) because the
// consumer is an upstream vision provider fetching the URL — it has no JWT or
// sk- header. Access is gated solely by the HMAC signature binding key+expires,
// plus an independent expiry check. The key travels in a catch-all path segment
// (it is "ab/<sha>"), and bytes stream straight from Storage so the file is
// never held fully in memory.
func GetVisionFile(c *gin.Context) {
	if service.UploadStore == nil {
		common.Error(c, http.StatusServiceUnavailable, "upload storage not initialized")
		return
	}
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		common.Error(c, http.StatusBadRequest, "missing key")
		return
	}

	expires, err := strconv.ParseInt(c.Query("expires"), 10, 64)
	if err != nil || c.Query("token") == "" {
		common.Error(c, http.StatusForbidden, "invalid signature")
		return
	}
	if time.Now().Unix() > expires {
		common.Error(c, http.StatusGone, "url expired")
		return
	}
	if !storage.VerifyURL(key, expires, c.Query("token")) {
		common.Error(c, http.StatusForbidden, "invalid signature")
		return
	}

	// Confirm the key is a tracked upload (rejects random/typo keys even with a
	// valid-looking signature, and catches rows already reaped by cleanup).
	img, err := model.GetUploadedImageByKey(key)
	if err != nil {
		common.Error(c, http.StatusNotFound, "file not found")
		return
	}

	rc, err := service.UploadStore.Get(c.Request.Context(), key)
	if err != nil {
		// Reaped on disk between the row lookup and here, or genuine I/O error.
		common.Error(c, http.StatusNotFound, "file not found")
		return
	}
	defer rc.Close()

	// private/no-store: signed URLs are per-issuer and short-lived; intermediaries
	// must not cache them. nosniff: respect the stored media type verbatim.
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, img.Size, img.MediaType, rc, nil)
}

// PutVisionFile receives raw image bytes for a presigned-PUT direct upload (the
// V1.1 shell/curl path). Mounted on the PUBLIC group alongside GetVisionFile:
// auth is the purpose-bound HMAC signature (VerifyURLFor "PUT"), NOT an API key
// — so the agent's curl carries no sk-. The key was minted at upload_image time
// (UUID, not content-addressed) and a pending row already exists; this flips it
// to uploaded. Used only by the local backend (S3 PUTs go straight to bucket).
func PutVisionFile(c *gin.Context) {
	if service.UploadStore == nil {
		common.Error(c, http.StatusServiceUnavailable, "upload storage not initialized")
		return
	}
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		common.Error(c, http.StatusBadRequest, "missing key")
		return
	}
	expires, err := strconv.ParseInt(c.Query("expires"), 10, 64)
	if err != nil || c.Query("token") == "" {
		common.Error(c, http.StatusForbidden, "invalid signature")
		return
	}
	if time.Now().Unix() > expires {
		common.Error(c, http.StatusGone, "url expired")
		return
	}
	if !storage.VerifyURLFor("PUT", key, expires, c.Query("token")) {
		common.Error(c, http.StatusForbidden, "invalid signature")
		return
	}

	// The slot must exist and still be pending (one-shot). UUID keys are globally
	// unique, so GetUploadedImageByKey finds the one pending slot.
	img, err := model.GetUploadedImageByKey(key)
	if err != nil {
		common.Error(c, http.StatusNotFound, "upload slot not found")
		return
	}
	if img.Status == model.UploadStatusUploaded {
		common.Error(c, http.StatusConflict, "upload slot already used")
		return
	}

	maxBytes := model.GetOptionInt64("VisionUploadMaxBytes")
	if maxBytes <= 0 {
		maxBytes = 10 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.Error(c, http.StatusRequestEntityTooLarge, "image exceeds size limit")
		return
	}

	mediaType := vision.SniffMediaType(data)
	if mediaType == "" {
		common.Error(c, http.StatusUnsupportedMediaType, "unrecognized image format")
		return
	}

	if err := service.UploadStore.Put(c.Request.Context(), key, bytes.NewReader(data), mediaType); err != nil {
		common.Error(c, http.StatusInternalServerError, "store failed")
		return
	}
	if err := model.MarkUploaded(key, mediaType, int64(len(data))); err != nil {
		common.Error(c, http.StatusInternalServerError, "record failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "uploaded", "key": key, "size": len(data), "mime": mediaType})
}

// actingUserID resolves the caller from either auth scheme (JWT user_id or
// API-key api_key_user_id), mirroring UploadVisionImage.
func actingUserID(c *gin.Context) int64 {
	uid := c.GetInt64("user_id")
	if uid == 0 {
		uid = c.GetInt64("api_key_user_id")
	}
	return uid
}

// uploadListItem is one row in the management list view. URL is re-signed on
// each read (signed URLs are short-lived); ExpiresAt is an estimate
// (created_at + retention) for display only.
type uploadListItem struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Key       string    `json:"key"`
	URL       string    `json:"url"`
	MediaType string    `json:"mime"`
	Size      int64     `json:"size"`
	Backend   string    `json:"backend"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func uploadSignedTTL() time.Duration {
	if s := model.GetOptionInt("SignedURLTTLSeconds"); s > 0 {
		return time.Duration(s) * time.Second
	}
	return time.Hour
}

func uploadRetentionDur() time.Duration {
	if h := model.GetOptionInt("UploadRetentionHours"); h > 0 {
		return time.Duration(h) * time.Hour
	}
	return 24 * time.Hour
}

func toUploadListItem(ctx context.Context, img model.UploadedImage) uploadListItem {
	url, _ := service.UploadStore.PublicURL(ctx, img.StorageKey, uploadSignedTTL())
	return uploadListItem{
		ID:        img.ID,
		UserID:    img.UserID,
		Key:       img.StorageKey,
		URL:       url,
		MediaType: img.MediaType,
		Size:      img.Size,
		Backend:   img.Backend,
		Status:    img.Status,
		CreatedAt: img.CreatedAt,
		ExpiresAt: img.CreatedAt.Add(uploadRetentionDur()),
	}
}

// ListUploads lists the caller's own uploaded images (uploaded only; pending
// slots are hidden). Mounted on both JWT (/vision/uploads) and API-key
// (/vision/mcp-uploads) routes. URLs are re-signed on each read.
func ListUploads(c *gin.Context) {
	if service.UploadStore == nil {
		common.Error(c, http.StatusServiceUnavailable, "upload storage not initialized")
		return
	}
	uid := actingUserID(c)
	if uid == 0 {
		common.Error(c, http.StatusUnauthorized, "missing user identity")
		return
	}
	page, pageSize := common.GetPagination(c)
	imgs, total, err := model.ListUploadsByUser(uid, common.GetOffset(page, pageSize), pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "list uploads failed")
		return
	}
	items := make([]uploadListItem, 0, len(imgs))
	for _, img := range imgs {
		items = append(items, toUploadListItem(c.Request.Context(), img))
	}
	common.PageOf(c, items, page, pageSize, total)
}

// DeleteUpload deletes one of the caller's own uploads (owner-checked; a
// non-owned id returns 404 to avoid leaking existence).
func DeleteUpload(c *gin.Context) {
	if service.UploadStore == nil {
		common.Error(c, http.StatusServiceUnavailable, "upload storage not initialized")
		return
	}
	uid := actingUserID(c)
	if uid == 0 {
		common.Error(c, http.StatusUnauthorized, "missing user identity")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := deleteUploadRow(c.Request.Context(), id, uid); err != nil {
		common.Error(c, http.StatusNotFound, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// AdminListUploads lists ALL users' uploads (optionally filtered by ?user_id=),
// for abuse handling / maintenance.
func AdminListUploads(c *gin.Context) {
	if service.UploadStore == nil {
		common.Error(c, http.StatusServiceUnavailable, "upload storage not initialized")
		return
	}
	page, pageSize := common.GetPagination(c)
	var filterUserID int64
	if v := c.Query("user_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filterUserID = id
		}
	}
	imgs, total, err := model.ListAllUploads(filterUserID, common.GetOffset(page, pageSize), pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "list uploads failed")
		return
	}
	items := make([]uploadListItem, 0, len(imgs))
	for _, img := range imgs {
		items = append(items, toUploadListItem(c.Request.Context(), img))
	}
	common.PageOf(c, items, page, pageSize, total)
}

// AdminDeleteUpload deletes any user's upload (no owner check).
func AdminDeleteUpload(c *gin.Context) {
	if service.UploadStore == nil {
		common.Error(c, http.StatusServiceUnavailable, "upload storage not initialized")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := deleteUploadRow(c.Request.Context(), id, 0); err != nil {
		common.Error(c, http.StatusNotFound, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// deleteUploadRow deletes a row and refcounts its shared blob. If ownerCheck > 0,
// the row must belong to that user or it's treated as not found (no existence
// leak). The blob is removed only when the last referencing row is gone.
func deleteUploadRow(ctx context.Context, id, ownerCheck int64) error {
	img, err := model.GetUploadedImageByID(id)
	if err != nil {
		return fmt.Errorf("upload not found")
	}
	if ownerCheck > 0 && img.UserID != ownerCheck {
		return fmt.Errorf("upload not found") // don't leak existence
	}
	if err := model.DeleteUploadedImageByID(img.ID); err != nil {
		return fmt.Errorf("delete failed")
	}
	if n, err := model.CountByKey(img.StorageKey); err == nil && n == 0 {
		_ = service.UploadStore.Delete(ctx, img.StorageKey)
	}
	return nil
}
