package vision

import "bytes"

// SniffMediaType identifies a common image format from its magic bytes. The
// bytes are authoritative — far more reliable than any label the sender
// attached. Returns "" for an unrecognized format so callers can reject early
// rather than burn an upstream request on garbage.
//
// Shared between the upload path (service.UploadService) and the vision tool
// handler (virtual.DecodeImage); both need to turn arbitrary image input into a
// trusted media type, so it lives here in the vision package both already
// depend on.
func SniffMediaType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}): // \x89PNG
		return "image/png"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "image/gif"
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	}
	return ""
}
