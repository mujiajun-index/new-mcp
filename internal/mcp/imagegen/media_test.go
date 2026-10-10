package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mediaPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestReferenceEncodingAndInspection(t *testing.T) {
	data := mediaPNG(t)
	for _, ref := range []string{base64.StdEncoding.EncodeToString(data), "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), base64.RawURLEncoding.EncodeToString(data)} {
		got, err := LoadReference(context.Background(), nil, 1, ref, 1024)
		if err != nil || !bytes.Equal(data, got.Bytes) || got.MIME != "image/png" {
			t.Fatalf("%v %v", got, err)
		}
	}
	if _, _, _, err := Inspect(data, 10); err == nil {
		t.Fatal("accepted over-limit image")
	}
	if _, _, _, err := Inspect([]byte("<html>not an image</html>"), 1024); err == nil {
		t.Fatal("accepted non-image")
	}
}

func TestDownloadPrivateAddressesRedirectsAndLimits(t *testing.T) {
	data := mediaPNG(t)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed redirect to unrelated private host") }))
	defer other.Close()
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "" || r.Header.Get("x-goog-api-key") != "" {
			t.Error("leaked model authentication")
		}
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, other.URL, http.StatusFound)
		case "/same":
			http.Redirect(w, r, "/image", http.StatusFound)
		default:
			w.Write(data)
		}
	}))
	defer server.Close()
	if _, err := Download(context.Background(), server.URL+"/image", "", 1024); err == nil || hits != 0 {
		t.Fatalf("reference SSRF allowed: %v", err)
	}
	got, err := Download(context.Background(), server.URL+"/same", server.URL+"/v1", 1024)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal(err)
	}
	if _, err := Download(context.Background(), server.URL+"/image", server.URL, 10); err == nil {
		t.Fatal("ignored download limit")
	}
	if _, err := Download(context.Background(), server.URL+"/redirect", server.URL, 1024); err == nil {
		t.Fatal("followed private redirect")
	}
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "100.100.100.200", "::1", "fc00::1", "::ffff:127.0.0.1"} {
		if publicIP(net.ParseIP(ip)) {
			t.Errorf("accepted %s", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIP(net.ParseIP(ip)) {
			t.Errorf("blocked public %s", ip)
		}
	}
}
