package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
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
