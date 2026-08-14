package vision

// Read-path image transforms (resize / high-fidelity re-encode), applied to
// in-memory bytes between reading the upload and base64-encoding for the
// upstream call. This realizes the "网关侧 resize (可选降本)" item from
// docs/VISION-IMAGE-TRANSPORT.md §13.2. Both transforms are gated by
// system-settings toggles (read live by the caller in package virtual and passed
// in as TransformOpts), default OFF, and only ever operate on the in-memory
// bytes — the on-disk original, the GET serve path, and the sha256 content-key
// dedup are never touched.
//
// Design invariants (see the plan):
//   - input.MediaType must always match the returned bytes; a transform that
//     changes format (WebP→JPEG) rewrites MediaType in lockstep, or the upstream
//     mime_type / data-URL would mismatch the bytes (client.go OpenAI/Anthropic/
//     Gemini encoders all trust MediaType verbatim).
//   - URL-passthrough inputs (in.IsURL()) are returned unchanged — new-mcp never
//     downloads those, so there is nothing to transform.
//   - FAIL-OPEN: every decode/encode error falls back to the original input. An
//     optional optimization must never block recognition (mirrors the project's
//     BillingFailOpen posture). The function therefore returns no error.
//   - Zero overhead when both toggles are off: the fast path returns before any
//     image.Decode / DecodeConfig.

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"math"

	_ "image/gif"               // side-effect: register gif decode (so DecodeConfig works for GIFs; GIF is then skipped)
	_ "golang.org/x/image/webp" // side-effect: register webp decode — REQUIRED; removing this makes WebP return "image: unknown format"

	"golang.org/x/image/draw"
)

// maxDecodePixels caps the decoded pixel buffer to bound per-request memory.
// image.Decode allocates width*height*4 bytes; a pathological image within the
// byte cap could otherwise OOM the gateway under concurrency. ~50M pixels
// (~200MB RGBA) comfortably covers a default 10MB-upload photo (≈12MP) while
// rejecting pathological dimensions. Oversized images fail-open to the original.
const maxDecodePixels = 50_000_000

// TransformOpts is the resolved, pure-data transform configuration. The caller
// (package virtual) reads the option keys and hands this in, keeping package
// vision free of any model/option import (it has none today).
type TransformOpts struct {
	ResizeEnabled   bool // downscale images whose long edge exceeds ResizeMaxEdge
	ResizeMaxEdge   int  // long-edge pixel threshold (aspect ratio preserved)
	CompressEnabled bool // re-encode JPEG/WebP at JPEGQuality to shrink bytes
	JPEGQuality     int  // 1-100, applied to any JPEG-family re-encode
}

// NewTransformOpts builds TransformOpts from raw option values, clamping the
// numeric fields to safe ranges (ResizeMaxEdge ≤ 0 → 1568, matching the upstream
// internal long-edge cap; JPEGQuality clamped to 1-100). Callers pass the values
// read straight from the options table (model.GetOptionBool / GetOptionInt), so
// the clamping rule lives in one place instead of being duplicated per call site
// (the Analyze path in package virtual and the admin preview in controller).
func NewTransformOpts(resizeEnabled bool, resizeMaxEdge int, compressEnabled bool, jpegQuality int) TransformOpts {
	if resizeMaxEdge <= 0 {
		resizeMaxEdge = 1568
	}
	if jpegQuality < 1 {
		jpegQuality = 1
	} else if jpegQuality > 100 {
		jpegQuality = 100
	}
	return TransformOpts{
		ResizeEnabled:   resizeEnabled,
		ResizeMaxEdge:   resizeMaxEdge,
		CompressEnabled: compressEnabled,
		JPEGQuality:     jpegQuality,
	}
}

// Dimensions returns the pixel dimensions of an encoded image by reading only its
// header (no full pixel decode). Used by the admin transform-preview endpoint to
// report before/after stats. Returns an error for unrecognized/corrupt input.
func Dimensions(b []byte) (width, height int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// Transform applies the configured resize/re-encode transforms to in.Bytes and
// returns the resulting ImageInput. See the package doc for the full contract.
// On every off-path and error case it returns the original input unchanged.
func Transform(in ImageInput, opts TransformOpts) ImageInput {
	// URL passthrough: new-mcp doesn't download these — nothing to transform.
	if in.IsURL() {
		return in
	}
	// Zero-overhead off path: both toggles closed → identical to today.
	if !opts.ResizeEnabled && !opts.CompressEnabled {
		return in
	}
	// GIF: stdlib gif.Decode returns only the first frame, so any transform drops
	// animation. Only the compress path converts GIF→JPEG (the admin opted into the
	// bandwidth saving knowing animation is lost); resize-only still skips GIF.
	if in.MediaType == "image/gif" && !opts.CompressEnabled {
		return in
	}

	// Read only the header (no pixel buffer) to get dimensions and guard memory.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(in.Bytes))
	if err != nil {
		log.Printf("[vision] Transform: decode config failed (%s); returning original: %v", in.MediaType, err)
		return in
	}
	if pixels := int64(cfg.Width) * int64(cfg.Height); pixels > maxDecodePixels {
		log.Printf("[vision] Transform: image %dx%d (%d px) exceeds decode cap %d; returning original", cfg.Width, cfg.Height, pixels, maxDecodePixels)
		return in
	}

	needResize := opts.ResizeEnabled && longEdge(cfg.Width, cfg.Height) > opts.ResizeMaxEdge

	// Output format policy:
	//   - Compress on → JPEG for EVERY input (jpeg/webp/png/gif → jpeg). This is the
	//     deliberate change from the original lossless-PNG / skip-GIF design: the
	//     admin opted into maximum bandwidth savings via JPEG re-encode. PNG/GIF
	//     transparency is flattened onto opaque white (see below).
	//   - Compress off (resize only) → keep the input format, except WebP and any
	//     non-PNG type, which have no lossless stdlib encoder and so become JPEG.
	//     GIF was skipped just above.
	var outType string
	switch {
	case opts.CompressEnabled:
		outType = "image/jpeg"
	case in.MediaType == "image/png":
		outType = "image/png" // resize-only: keep PNG (lossless, alpha preserved)
	default:
		outType = "image/jpeg" // jpeg stays jpeg; webp/unknown → jpeg
	}
	outIsJPEG := outType == "image/jpeg"

	// Resize on but already under threshold, and not compressing → nothing to do.
	// Avoids a full decode for the common small upload.
	if !needResize && !opts.CompressEnabled {
		return in
	}

	src, _, err := image.Decode(bytes.NewReader(in.Bytes))
	if err != nil {
		log.Printf("[vision] Transform: full decode failed (%s); returning original: %v", in.MediaType, err)
		return in
	}

	// Produce the image to encode: resize if needed, and flatten transparency onto
	// opaque white for JPEG-family output (JPEG has no alpha — without this, PNG/GIF
	// transparent regions would render black/undefined).
	work := src
	if needResize {
		work = scaleDown(src, opts.ResizeMaxEdge, outIsJPEG)
	} else if outIsJPEG {
		// Compress-only (no resize) on a possibly-alpha image: flatten before encode.
		work = flattenAlphaForJPEG(src)
	}

	var outBytes []byte
	if outIsJPEG {
		buf, encErr := encodeJPEG(work, opts.JPEGQuality)
		if encErr != nil {
			log.Printf("[vision] Transform: jpeg encode failed; returning original: %v", encErr)
			return in
		}
		outBytes = buf
	} else { // png
		buf, encErr := encodePNG(work)
		if encErr != nil {
			log.Printf("[vision] Transform: png encode failed; returning original: %v", encErr)
			return in
		}
		outBytes = buf
	}

	// If the transform produced larger bytes (e.g. converting a tiny/flat PNG or a
	// paletted GIF to JPEG), keep the original — the goal is to shrink, never inflate.
	if len(outBytes) >= len(in.Bytes) {
		return in
	}
	return ImageInput{Bytes: outBytes, MediaType: outType}
}

func longEdge(w, h int) int {
	if h > w {
		return h
	}
	return w
}

// scaleDown resizes src so its long edge equals maxEdge, preserving aspect
// ratio. When flattenWhite is set, transparent source pixels are composited onto
// opaque white — use this for JPEG-family output, which has no alpha channel.
func scaleDown(src image.Image, maxEdge int, flattenWhite bool) image.Image {
	b := src.Bounds()
	long := longEdge(b.Dx(), b.Dy())
	if long <= maxEdge {
		return src // defensive; caller already guards needResize
	}
	scale := float64(maxEdge) / float64(long)
	newW := max(1, int(math.Round(float64(b.Dx())*scale)))
	newH := max(1, int(math.Round(float64(b.Dy())*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	op := draw.Src
	if flattenWhite {
		// Pre-fill white, then composite the scaled source over it so transparent
		// regions render white instead of black.
		draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, dst.Bounds().Min, draw.Src)
		op = draw.Over
	}
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, op, nil)
	return dst
}

// flattenAlphaForJPEG returns an image safe for JPEG encoding. Opaque image types
// (YCbCr/Gray from a decoded JPEG) are returned unchanged; alpha-capable types
// are composited onto opaque white so transparent regions don't render black.
func flattenAlphaForJPEG(img image.Image) image.Image {
	switch img.(type) {
	case *image.NRGBA, *image.RGBA, *image.Paletted, *image.Uniform:
		// these can carry a meaningful alpha channel
	default:
		return img
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, &image.Uniform{C: color.White}, b.Min, draw.Src)
	draw.Draw(dst, b, img, b.Min, draw.Over)
	return dst
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
