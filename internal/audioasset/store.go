// Package audioasset stores uploaded, reference and generated audio in the shared blob backend.
package audioasset

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

func Option(key string, fallback int64) int64 {
	if model.GetOptionString(key) == "" {
		return fallback
	}
	return model.GetOptionInt64(key)
}
func InputLimit() int64  { return max(1, Option("AudioGenInputMaxBytes", 25<<20)) }
func OutputLimit() int64 { return max(1, Option("AudioGenMaxBytes", 128<<20)) }
func Expired(a *model.AudioAsset) bool {
	return a.Status == "expired" || (Option("AudioGenRetentionHours", 24) != 0 && a.ExpiresAt != nil && !a.ExpiresAt.After(time.Now()))
}
func Sign(method, sid string) string {
	secret := common.SessionSecret
	if secret == "" {
		secret = "default-secret-change-me"
	}
	if method == "HEAD" {
		method = "GET"
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("audio|" + method + "|" + sid))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:12])
}
func Verify(method, sid, token string) bool {
	return token != "" && hmac.Equal([]byte(token), []byte(Sign(method, sid)))
}
func URL(method, sid string) string {
	return strings.TrimRight(model.GetOptionString("ServerAddress"), "/") + "/a/" + sid + "?s=" + Sign(method, sid)
}
func Read(r io.Reader, limit int64) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("audio exceeds %d bytes", limit)
	}
	return b, nil
}

// Inspect checks the container, not a caller-supplied suffix or Content-Type.
func Inspect(b []byte, input bool) (string, string, error) {
	if len(b) < 12 {
		return "", "", fmt.Errorf("invalid or empty audio")
	}
	if string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE" {
		if int64(binary.LittleEndian.Uint32(b[4:8]))+8 > int64(len(b)) {
			return "", "", fmt.Errorf("truncated WAV container")
		}
		foundFmt, foundData := false, false
		for pos := 12; pos+8 <= len(b); {
			n := int(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
			if n < 0 || n > len(b)-pos-8 {
				return "", "", fmt.Errorf("truncated WAV chunk")
			}
			switch string(b[pos : pos+4]) {
			case "fmt ":
				if n < 16 {
					return "", "", fmt.Errorf("invalid WAV format")
				}
				foundFmt = true
			case "data":
				foundData = n > 0
			}
			pos += 8 + n + (n % 2)
		}
		if !foundFmt || !foundData {
			return "", "", fmt.Errorf("WAV requires format and audio data")
		}
		return "audio/wav", "wav", nil
	}
	pos := 0
	if string(b[:3]) == "ID3" {
		if b[6]|b[7]|b[8]|b[9] >= 128 {
			return "", "", fmt.Errorf("invalid MP3 tag")
		}
		pos = 10 + (int(b[6])<<21 | int(b[7])<<14 | int(b[8])<<7 | int(b[9]))
		if b[5]&0x10 != 0 {
			pos += 10
		}
	}
	for i := pos; i+4 < len(b) && i < pos+4096; i++ {
		if b[i] == 255 && b[i+1]&0xe0 == 0xe0 && b[i+1]&0x18 != 8 && b[i+1]&6 != 0 && b[i+2]&0xf0 != 0 && b[i+2]&0xf0 != 0xf0 && b[i+2]&0x0c != 0x0c {
			// Require a complete Layer III frame, not only a coincidental sync word.
			if b[i+1]&6 != 2 {
				continue
			}
			version := (b[i+1] >> 3) & 3
			rate := []int{44100, 48000, 32000}[(b[i+2]>>2)&3]
			table := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
			factor := 144000
			if version != 3 {
				table = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
				factor = 72000
				rate /= 2
				if version == 0 {
					rate /= 2
				}
			}
			frame := factor*table[b[i+2]>>4]/rate + int((b[i+2]>>1)&1)
			if frame >= 4 && len(b)-i >= frame {
				return "audio/mpeg", "mp3", nil
			}
		}
	}
	if !input {
		switch string(b[:4]) {
		case "fLaC":
			return "audio/flac", "flac", nil
		case "OggS":
			return "audio/ogg", "ogg", nil
		}
		if len(b) > 12 && string(b[4:8]) == "ftyp" {
			return "audio/mp4", "m4a", nil
		}
		if b[0] == 255 && b[1]&0xf6 == 0xf0 {
			return "audio/aac", "aac", nil
		}
	}
	return "", "", fmt.Errorf("unrecognized audio; input must be WAV or MP3")
}
func WAV(pcm []byte, rate, channels, bits int) ([]byte, error) {
	if rate <= 0 || channels < 1 || channels > 8 || bits != 16 || len(pcm) == 0 || len(pcm)%(channels*2) != 0 {
		return nil, fmt.Errorf("PCM requires known sample rate, channels and 16-bit aligned samples")
	}
	b := make([]byte, 44+len(pcm))
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+len(pcm)))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], uint16(channels))
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(b[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(len(pcm)))
	copy(b[44:], pcm)
	return b, nil
}
func NewPending(db *gorm.DB, uid int64) (*model.AudioAsset, error) {
	maxUploads := model.GetOptionInt64("MaxUploadsPerUser")
	if maxUploads > 0 {
		var count int64
		if e := db.Model(&model.AudioAsset{}).Where("user_id = ? AND source = ? AND status IN ?", uid, "upload", []string{"pending", "uploading", "ready"}).Count(&count).Error; e != nil {
			return nil, e
		}
		if count >= maxUploads {
			return nil, fmt.Errorf("audio upload quota exceeded")
		}
	}
	sid, e := model.NewShortID()
	if e != nil {
		return nil, e
	}
	a := &model.AudioAsset{UserID: uid, ShortID: sid, StorageKey: "audio/" + uuid.NewString(), Source: "upload", Status: "pending"}
	e = db.Create(a).Error
	return a, e
}
func Save(ctx context.Context, db *gorm.DB, store storage.Storage, uid int64, taskID, source string, b []byte) (*model.AudioAsset, error) {
	if store == nil {
		return nil, fmt.Errorf("audio storage not initialized")
	}
	mime, _, e := Inspect(b, source != "generated")
	if e != nil {
		return nil, e
	}
	limit := OutputLimit()
	if source != "generated" {
		limit = InputLimit()
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("audio exceeds size limit")
	}
	sid, e := model.NewShortID()
	if e != nil {
		return nil, e
	}
	a := &model.AudioAsset{UserID: uid, TaskID: taskID, ShortID: sid, StorageKey: "audio/" + uuid.NewString(), Source: source, Status: "ready", MIME: mime, Size: int64(len(b))}
	if hours := Option("AudioGenRetentionHours", 24); hours > 0 {
		expires := time.Now().Add(time.Duration(hours) * time.Hour)
		a.ExpiresAt = &expires
	}
	if e = store.Put(ctx, a.StorageKey, bytes.NewReader(b), mime); e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		_ = store.Delete(context.Background(), a.StorageKey)
		return nil, e
	}
	if e = db.Create(a).Error; e != nil {
		_ = store.Delete(ctx, a.StorageKey)
		return nil, e
	}
	return a, nil
}
func Load(ctx context.Context, db *gorm.DB, store storage.Storage, uid int64, ref string, limit int64) ([]byte, string, error) {
	ref = strings.TrimSpace(ref)
	var b []byte
	var err error
	u, e := url.Parse(ref)
	base, _ := url.Parse(model.GetOptionString("ServerAddress"))
	sid, own := "", false
	if e == nil && (u.Scheme == "http" || u.Scheme == "https") {
		clean := path.Clean(u.Path)
		sid = path.Base(clean)
		if path.Base(path.Dir(clean)) == "a" {
			// A valid local capability remains local through alternate hostnames,
			// default ports or proxy path normalization; never bypass ownership by refetching it.
			own = Verify("GET", sid, u.Query().Get("s")) || (base != nil && strings.EqualFold(u.Hostname(), base.Hostname()))
		}
	}
	if own {
		if !Verify("GET", sid, u.Query().Get("s")) {
			return nil, "", fmt.Errorf("invalid audio signature")
		}
		var a model.AudioAsset
		if db.Where("short_id = ? AND user_id = ? AND status = ?", sid, uid, "ready").First(&a).Error != nil || Expired(&a) {
			return nil, "", fmt.Errorf("audio unavailable, expired or belongs to another user")
		}
		if a.Size > limit {
			return nil, "", fmt.Errorf("audio exceeds input limit")
		}
		r, e := store.Get(ctx, a.StorageKey)
		if e != nil {
			return nil, "", e
		}
		defer r.Close()
		b, err = Read(r, limit)
	} else if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
		b, err = imagegen.Download(ctx, ref, "", limit)
	} else {
		if strings.HasPrefix(ref, "data:") {
			i := strings.Index(ref, ",")
			if i < 0 || !strings.HasSuffix(ref[:i], ";base64") {
				return nil, "", fmt.Errorf("invalid audio data URL")
			}
			ref = ref[i+1:]
		}
		if int64(len(ref)) > ((limit+2)/3)*4+4 {
			return nil, "", fmt.Errorf("audio exceeds input limit")
		}
		b, err = base64.StdEncoding.DecodeString(ref)
	}
	if err != nil {
		return nil, "", err
	}
	_, format, err := Inspect(b, true)
	return b, format, err
}
func View(a *model.AudioAsset) map[string]any {
	status := a.Status
	if Expired(a) {
		status = "expired"
	}
	v := map[string]any{"id": a.ID, "mime": a.MIME, "size": a.Size, "status": status, "expires_at": a.ExpiresAt}
	if status == "ready" {
		v["url"] = URL("GET", a.ShortID)
	}
	return v
}
