package editor_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"scanexercise/internal/editor"
)

func jpegBytes(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func successPayload(image []byte) []byte {
	body, _ := json.Marshal(map[string]any{
		"code":    200,
		"message": "success",
		"result":  map[string]any{"image": base64.StdEncoding.EncodeToString(image)},
	})
	return body
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestEditPostsBinaryHandwrittenErase(t *testing.T) {
	source := jpegBytes(40, 60)
	var seenURL string
	var seen http.Header
	var seenBody []byte
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenURL = req.URL.String()
		seen = req.Header.Clone()
		seenBody, _ = io.ReadAll(req.Body)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(successPayload(jpegBytes(30, 40)))),
			Header:     make(http.Header),
		}, nil
	})}
	ed := editor.NewTextIn("app-id", "secret-code", 30)
	ed.Client = client
	result, err := ed.Edit(source, editor.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(seenURL, "https://api.textin.com/ai/service/v1/handwritten_erase?") {
		t.Fatalf("url %s", seenURL)
	}
	for _, part := range []string{"crop=0", "dewarp=0", "binarization=0", "image_type=1", "doc_direction=0"} {
		if !strings.Contains(seenURL, part) {
			t.Fatalf("missing %s in %s", part, seenURL)
		}
	}
	if seen.Get("x-ti-app-id") != "app-id" || seen.Get("x-ti-secret-code") != "secret-code" {
		t.Fatalf("headers %#v", seen)
	}
	if seen.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("content-type %s", seen.Get("Content-Type"))
	}
	if len(seenBody) < 2 || seenBody[0] != 0xFF || seenBody[1] != 0xD8 {
		t.Fatal("textin body is not jpeg")
	}
	if len(result) < 2 || result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("result is not jpeg")
	}
}

func TestEditSendsCropWhenEnabled(t *testing.T) {
	var seenURL string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenURL = req.URL.String()
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(successPayload(jpegBytes(20, 20)))),
			Header:     make(http.Header),
		}, nil
	})}
	ed := editor.NewTextIn("app-id", "secret-code", 30)
	ed.Client = client
	if _, err := ed.Edit(jpegBytes(8, 8), editor.Options{Crop: 1}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenURL, "crop=1") {
		t.Fatalf("url %s", seenURL)
	}
	if !strings.Contains(seenURL, "binarization=0") {
		t.Fatalf("url %s", seenURL)
	}
}

func TestEditSendsBinarizationWhenEnabled(t *testing.T) {
	var seenURL string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenURL = req.URL.String()
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(successPayload(jpegBytes(20, 20)))),
			Header:     make(http.Header),
		}, nil
	})}
	ed := editor.NewTextIn("app-id", "secret-code", 30)
	ed.Client = client
	if _, err := ed.Edit(jpegBytes(8, 8), editor.Options{Binarization: 1}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenURL, "binarization=1") {
		t.Fatalf("url %s", seenURL)
	}
	if !strings.Contains(seenURL, "crop=0") {
		t.Fatalf("url %s", seenURL)
	}
}

func TestMissingCredentialsFailBeforeTheRequest(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("should not request %s", req.URL)
		return nil, nil
	})}
	emptyApp := editor.NewTextIn("", "sk", 5)
	emptyApp.Client = client
	if _, err := emptyApp.Edit(jpegBytes(8, 8), editor.Options{}); err == nil || !strings.Contains(err.Error(), "TEXTIN_APP_ID") {
		t.Fatalf("app id error: %v", err)
	}
	emptySecret := editor.NewTextIn("ak", "", 5)
	emptySecret.Client = client
	if _, err := emptySecret.Edit(jpegBytes(8, 8), editor.Options{}); err == nil || !strings.Contains(err.Error(), "TEXTIN_SECRET_CODE") {
		t.Fatalf("secret error: %v", err)
	}
}

func TestTextInErrorIsLoggedWithoutSecrets(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	ed := editor.NewTextIn("app-id", "secret-code", 5)
	ed.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{
			"code":         40306,
			"message":      "qps limit",
			"x_request_id": "req-9",
			"duration":     12,
			"result":       map[string]any{"image": "secret-image-bytes"},
		})
		header := make(http.Header)
		header.Set("X-Request-Id", "header-should-not-override")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: header}, nil
	})}
	if _, err := ed.Edit(jpegBytes(8, 8), editor.Options{}); err == nil || err.Error() != "model call failed" {
		t.Fatalf("err %v", err)
	}
	line := logs.String()
	for _, part := range []string{"textin error", "status=200", "code=40306", "message=\"qps limit\"", "request_id=req-9", "duration=12", "[redacted]"} {
		if !strings.Contains(line, part) {
			t.Fatalf("log missing %s: %s", part, line)
		}
	}
	if strings.Contains(line, "secret-code") || strings.Contains(line, "secret-image-bytes") || strings.Contains(line, "app-id") {
		t.Fatalf("secret in log: %s", line)
	}
}

func TestAPIErrorAndMissingImageAreFailures(t *testing.T) {
	reject := editor.NewTextIn("ak", "sk", 5)
	reject.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{"code": 40102, "message": "invalid key"})
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
	if _, err := reject.Edit(jpegBytes(8, 8), editor.Options{}); err == nil || err.Error() != "model call failed" {
		t.Fatalf("reject: %v", err)
	}

	empty := editor.NewTextIn("ak", "sk", 5)
	empty.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{"code": 200, "result": map[string]any{}})
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
	if _, err := empty.Edit(jpegBytes(8, 8), editor.Options{}); err == nil || err.Error() != "model returned no image" {
		t.Fatalf("empty: %v", err)
	}
}
