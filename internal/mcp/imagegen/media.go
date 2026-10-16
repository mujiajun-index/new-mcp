package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	_ "golang.org/x/image/webp"
)

func MaxImageBytes() int64 {
	if max := model.GetOptionInt64("VisionUploadMaxBytes"); max > 0 {
		return max
	}
	return 10 << 20
}
func Inspect(data []byte, max int64) (Image, int, int, error) {
	if int64(len(data)) > max {
		return Image{}, 0, 0, fmt.Errorf("image exceeds %d bytes", max)
	}
	mime := vision.SniffMediaType(data)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" {
		return Image{}, 0, 0, fmt.Errorf("image must be PNG, JPEG or WebP")
	}
	dimensions, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Image{}, 0, 0, fmt.Errorf("invalid image: %w", err)
	}
	return Image{Bytes: data, MIME: mime}, dimensions.Width, dimensions.Height, nil
}

func LoadReference(ctx context.Context, store storage.Storage, userID int64, ref string, max int64) (Image, error) {
	ref = strings.TrimSpace(ref)
	var data []byte
	var err error
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		if storage.OwnsShortURL(ref) {
			sid, ok := storage.ShortIDFromURL(ref)
			u, e := url.Parse(ref)
			if !ok || e != nil || !storage.VerifyShort("GET", sid, u.Query().Get("s")) {
				return Image{}, fmt.Errorf("invalid storage image signature")
			}
			row, e := model.GetUploadedImageByShortID(sid)
			if e != nil || row.UserID != userID || row.Status != model.UploadStatusUploaded {
				return Image{}, fmt.Errorf("storage image is unavailable or belongs to another user")
			}
			if imageasset.Expired(row.CreatedAt) {
				return Image{}, fmt.Errorf("storage image has expired")
			}
			if row.Size > max {
				return Image{}, fmt.Errorf("image exceeds %d bytes", max)
			}
			if store == nil {
				return Image{}, fmt.Errorf("image storage not initialized")
			}
			reader, e := store.Get(ctx, row.StorageKey)
			if e != nil {
				return Image{}, e
			}
			defer reader.Close()
			data, err = readBounded(reader, max)
		} else {
			data, err = Download(ctx, ref, "", max)
		}
	} else {
		if strings.HasPrefix(ref, "data:") {
			i := strings.Index(ref, ",")
			if i < 0 || !strings.HasSuffix(ref[:i], ";base64") {
				return Image{}, fmt.Errorf("invalid image data URL")
			}
			ref = ref[i+1:]
		}
		if int64(len(ref)) > ((max+2)/3)*4+4 {
			return Image{}, fmt.Errorf("base64 image exceeds %d bytes", max)
		}
		for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			data, err = encoding.DecodeString(ref)
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		return Image{}, err
	}
	img, _, _, err := Inspect(data, max)
	return img, err
}
func readBounded(reader io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("image exceeds %d bytes", max)
	}
	return data, nil
}

// Download pins resolved IPs at dial time, validates redirects, ignores proxy
// environment variables, and never attaches upstream authentication headers.
// Only generated output can opt into the explicitly configured gateway origin.
func Download(ctx context.Context, raw, trustedBase string, max int64) ([]byte, error) {
	return downloadWithLookup(ctx, raw, trustedBase, max, lookupImageHost)
}

func downloadWithLookup(ctx context.Context, raw, trustedBase string, max int64, lookup func(context.Context, string, bool) ([]net.IPAddr, error)) ([]byte, error) {
	resp, err := openDownloadWithLookup(ctx, raw, trustedBase, max, "", lookup)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readBounded(resp.Body, max)
}

// OpenDownload shares the pinned-DNS and redirect protections with image
// downloads, but leaves bounded streaming to the media consumer. Authentication
// is restricted to the explicitly configured origin, including redirects.
func OpenDownload(ctx context.Context, raw, trustedBase string, max int64, authorization string) (*http.Response, error) {
	return openDownloadWithLookup(ctx, raw, trustedBase, max, authorization, lookupImageHost)
}

func openDownloadWithLookup(ctx context.Context, raw, trustedBase string, max int64, authorization string, lookup func(context.Context, string, bool) ([]net.IPAddr, error)) (*http.Response, error) {
	var trusted *url.URL
	if trustedBase != "" {
		trusted, _ = url.Parse(trustedBase)
	}
	target, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid image URL")
	}
	if !target.IsAbs() && trusted != nil {
		target = trusted.ResolveReference(target)
	}
	valid := func(u *url.URL) error {
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return fmt.Errorf("image URL must use http(s) without credentials")
		}
		return nil
	}
	if err = valid(target); err != nil {
		return nil, err
	}
	trustedOrigin := func(u *url.URL) bool {
		return trusted != nil && u.Scheme == trusted.Scheme && strings.EqualFold(u.Host, trusted.Host)
	}
	transport := &http.Transport{TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		allowPrivate := trustedOrigin(target) && strings.EqualFold(host, target.Hostname()) && port == effectivePort(target)
		ips, e := lookup(ctx, host, allowPrivate)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("image host has no addresses")
		}
		for _, entry := range ips {
			if !allowPrivate && !publicIP(entry.IP) {
				return nil, fmt.Errorf("image host %q resolves to prohibited network address %s", host, entry.IP)
			}
		}
		dialer := net.Dialer{Timeout: 10 * time.Second}
		var last error
		for _, entry := range ips {
			conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(entry.IP.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many image redirects")
		}
		if e := valid(req.URL); e != nil {
			return e
		}
		if !trustedOrigin(req.URL) {
			req.Header.Del("Authorization")
		}
		// DialContext's private-host exception is restricted to the original origin.
		if trustedOrigin(target) && !trustedOrigin(req.URL) && strings.EqualFold(req.URL.Hostname(), target.Hostname()) {
			return fmt.Errorf("image redirect changed gateway origin")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	if authorization != "" && trustedOrigin(target) {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := client.Do(req)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("image download HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > max {
		resp.Body.Close()
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("image exceeds %d bytes", max)
	}
	resp.Body = &downloadBody{ReadCloser: resp.Body, transport: transport}
	return resp, nil
}

type downloadBody struct {
	io.ReadCloser
	transport *http.Transport
}

func (b *downloadBody) Close() error {
	err := b.ReadCloser.Close()
	b.transport.CloseIdleConnections()
	return err
}
func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}
