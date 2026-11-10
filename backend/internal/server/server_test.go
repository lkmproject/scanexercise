package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"scanexercise/internal/apicode"
	"scanexercise/internal/auth"
	"scanexercise/internal/config"
	"scanexercise/internal/editor"
	"scanexercise/internal/jobs"
	appserver "scanexercise/internal/server"
)

type fakeEditor struct {
	results [][]byte
	err     error
	calls   [][]byte
	crops   []int
}

func (f *fakeEditor) Edit(jpegBytes []byte, opts editor.Options) ([]byte, error) {
	f.calls = append(f.calls, append([]byte(nil), jpegBytes...))
	f.crops = append(f.crops, opts.Crop)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.results) == 0 {
		return nil, editor.Fail("model returned no image")
	}
	out := f.results[0]
	f.results = f.results[1:]
	return out, nil
}

type failSecond struct {
	calls int
}

func (f *failSecond) Edit(jpegBytes []byte, opts editor.Options) ([]byte, error) {
	f.calls++
	if f.calls == 1 {
		return jpegColor(color.RGBA{G: 255, A: 255}, 20, 10), nil
	}
	return nil, editor.Fail("model timed out")
}

func jpegColor(c color.Color, width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func jpegWithOrientation(width, height int, orientation uint16) []byte {
	raw := jpegColor(color.RGBA{B: 255, A: 255}, width, height)
	exif := buildExifAPP1(orientation)
	out := append([]byte{}, raw[:2]...)
	out = append(out, exif...)
	out = append(out, raw[2:]...)
	return out
}

func buildExifAPP1(orientation uint16) []byte {
	payload := []byte("Exif\x00\x00")
	tiff := []byte{
		'I', 'I', 0x2A, 0x00,
		0x08, 0x00, 0x00, 0x00,
		0x01, 0x00,
		0x12, 0x01,
		0x03, 0x00,
		0x01, 0x00, 0x00, 0x00,
		byte(orientation), byte(orientation >> 8), 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	payload = append(payload, tiff...)
	length := len(payload) + 2
	app1 := []byte{0xFF, 0xE1, byte(length >> 8), byte(length)}
	return append(app1, payload...)
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (b *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(clone)
}

type memoryWallet struct {
	mu        sync.Mutex
	remaining int
}

func (m *memoryWallet) Reserve(_ context.Context, _ string, pages int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.remaining < pages {
		return auth.ErrInsufficient
	}
	m.remaining -= pages
	return nil
}

func (m *memoryWallet) Refund(_ context.Context, _ string, pages int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remaining += pages
	return nil
}

func (m *memoryWallet) Balance(_ context.Context, _ string) (int, error) {
	return m.remainingNow(), nil
}

func (m *memoryWallet) remainingNow() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.remaining
}

func newClient(t *testing.T, ed editor.Editor) (*httptest.Server, *http.Client) {
	return newClientWithWallet(t, ed, nil)
}

func newClientWithWallet(t *testing.T, ed editor.Editor, wallet jobs.Wallet) (*httptest.Server, *http.Client) {
	t.Helper()
	settings := config.Settings{
		DataDir:              t.TempDir(),
		MaxPages:             20,
		RetentionHours:       24,
		MaxLongEdge:          100,
		TextInTimeoutSeconds: 5,
		MaxUploadBytes:       20 * 1024 * 1024,
	}
	store := jobs.NewStore(settings, ed)
	store.SetWallet(wallet)
	tokens := auth.NewTokens("test-secret")
	token, err := tokens.Issue("test-user")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(appserver.New(store, nil, tokens))
	t.Cleanup(ts.Close)
	client := ts.Client()
	client.Transport = &bearerTransport{base: client.Transport, token: token}
	return ts, client
}

func postPages(t *testing.T, client *http.Client, base string, files [][]byte) *http.Response {
	t.Helper()
	return postPagesCrop(t, client, base, files, "")
}

func postPagesCrop(t *testing.T, client *http.Client, base string, files [][]byte, crop string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for index, file := range files {
		part, err := writer.CreateFormFile("pages", "page-"+string(rune('a'+index))+".jpg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file); err != nil {
			t.Fatal(err)
		}
	}
	if crop != "" {
		if err := writer.WriteField("crop", crop); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/jobs", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeEnvelope(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func decodeData(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	payload := decodeEnvelope(t, resp)
	if codeOf(payload) != int32(apicode.OK) {
		t.Fatalf("code %v reason %v", payload["code"], payload["reason"])
	}
	if payload["reason"] != "success" {
		t.Fatalf("reason %#v", payload["reason"])
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		t.Fatal("missing data")
	}
	return data
}

func codeOf(payload map[string]any) int32 {
	switch value := payload["code"].(type) {
	case float64:
		return int32(value)
	case int32:
		return value
	case int:
		return int32(value)
	default:
		return 0
	}
}

func remainingOf(data map[string]any) int {
	switch value := data["remaining"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return -1
	}
}

func wait(t *testing.T, client *http.Client, base, jobID string) map[string]any {
	t.Helper()
	var last map[string]any
	for range 50 {
		resp, err := client.Get(base + "/jobs/" + jobID)
		if err != nil {
			t.Fatal(err)
		}
		last = decodeData(t, resp)
		status, _ := last["status"].(string)
		if status == "succeeded" || status == "failed" {
			return last
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("job did not finish: %#v", last)
	return last
}

func TestPreprocessAppliesExifAndLimitsLongEdge(t *testing.T) {
	ed := &fakeEditor{results: [][]byte{jpegColor(color.RGBA{G: 255, A: 255}, 16, 16)}}
	ts, client := newClient(t, ed)
	raw := jpegWithOrientation(300, 40, 6)
	resp := postPages(t, client, ts.URL, [][]byte{raw})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	jobID := decodeData(t, resp)["id"].(string)
	if wait(t, client, ts.URL, jobID)["status"] != "succeeded" {
		t.Fatal("expected success")
	}
	prepared, err := jpeg.Decode(bytes.NewReader(ed.calls[0]))
	if err != nil {
		t.Fatal(err)
	}
	bounds := prepared.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if max(width, height) > 100 {
		t.Fatalf("long edge %d", max(width, height))
	}
	if width >= height {
		t.Fatalf("expected portrait after EXIF, got %dx%d", width, height)
	}
}

func TestSuccessReturnsEditedPagesInOrderThenDeletesAfterPDF(t *testing.T) {
	first := jpegColor(color.RGBA{R: 255, A: 255}, 30, 20)
	second := jpegColor(color.RGBA{B: 255, A: 255}, 30, 20)
	edited := [][]byte{
		jpegColor(color.RGBA{G: 255, A: 255}, 16, 16),
		jpegColor(color.RGBA{R: 255, G: 255, A: 255}, 18, 12),
	}
	ed := &fakeEditor{results: append([][]byte{}, edited...)}
	ts, client := newClient(t, ed)
	resp := postPages(t, client, ts.URL, [][]byte{first, second})
	jobID := decodeData(t, resp)["id"].(string)
	body := wait(t, client, ts.URL, jobID)
	if body["status"] != "succeeded" {
		t.Fatalf("status %v", body["status"])
	}
	previews, _ := body["previews"].([]any)
	if len(previews) != 2 || previews[0] != "/jobs/"+jobID+"/preview/0" || previews[1] != "/jobs/"+jobID+"/preview/1" {
		t.Fatalf("previews %#v", previews)
	}
	page0 := getBytes(t, client, ts.URL+"/jobs/"+jobID+"/preview/0")
	page1 := getBytes(t, client, ts.URL+"/jobs/"+jobID+"/preview/1")
	if !bytes.Equal(page0, edited[0]) || !bytes.Equal(page1, edited[1]) {
		t.Fatal("preview mismatch")
	}
	if bytes.Contains(page0, first) {
		t.Fatal("original leaked into preview")
	}
	pdfResp, err := client.Get(ts.URL + "/jobs/" + jobID + "/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer pdfResp.Body.Close()
	if pdfResp.StatusCode != http.StatusOK {
		t.Fatalf("pdf status %d", pdfResp.StatusCode)
	}
	if !strings.HasPrefix(pdfResp.Header.Get("Content-Type"), "application/pdf") {
		t.Fatalf("content-type %s", pdfResp.Header.Get("Content-Type"))
	}
	pdfBytes, _ := io.ReadAll(pdfResp.Body)
	if bytes.Index(pdfBytes, edited[0]) >= bytes.Index(pdfBytes, edited[1]) {
		t.Fatal("pdf page order")
	}
	missing, err := client.Get(ts.URL + "/jobs/" + jobID)
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeEnvelope(t, missing)
	if missing.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after pdf, got %d", missing.StatusCode)
	}
	if codeOf(payload) != int32(apicode.JobNotFound) {
		t.Fatalf("code %v reason %v", payload["code"], payload["reason"])
	}
}

func TestModelFailureDoesNotReturnTheOriginal(t *testing.T) {
	original := jpegColor(color.RGBA{R: 255, A: 255}, 20, 10)
	ed := &fakeEditor{err: editor.Fail("model returned no image")}
	ts, client := newClient(t, ed)
	resp := postPages(t, client, ts.URL, [][]byte{original})
	jobID := decodeData(t, resp)["id"].(string)
	body := wait(t, client, ts.URL, jobID)
	if body["status"] != "failed" || body["error"] != "model returned no image" {
		t.Fatalf("body %#v", body)
	}
	preview, err := client.Get(ts.URL + "/jobs/" + jobID + "/preview/0")
	if err != nil {
		t.Fatal(err)
	}
	previewPayload := decodeEnvelope(t, preview)
	if preview.StatusCode != http.StatusOK {
		t.Fatalf("preview status %d", preview.StatusCode)
	}
	if codeOf(previewPayload) != int32(apicode.JobFailed) {
		t.Fatalf("preview code %v", previewPayload["code"])
	}
	pdfResp, err := client.Get(ts.URL + "/jobs/" + jobID + "/pdf")
	if err != nil {
		t.Fatal(err)
	}
	pdfPayload := decodeEnvelope(t, pdfResp)
	if pdfResp.StatusCode != http.StatusOK {
		t.Fatalf("pdf status %d", pdfResp.StatusCode)
	}
	if codeOf(pdfPayload) != int32(apicode.JobFailed) {
		t.Fatalf("pdf code %v", pdfPayload["code"])
	}
}

func TestSecondPageFailureDiscardsPartialOutput(t *testing.T) {
	ts, client := newClient(t, &failSecond{})
	resp := postPages(t, client, ts.URL, [][]byte{
		jpegColor(color.RGBA{R: 255, A: 255}, 20, 10),
		jpegColor(color.RGBA{B: 255, A: 255}, 20, 10),
	})
	jobID := decodeData(t, resp)["id"].(string)
	body := wait(t, client, ts.URL, jobID)
	if body["status"] != "failed" || body["error"] != "model timed out" {
		t.Fatalf("body %#v", body)
	}
	preview, err := client.Get(ts.URL + "/jobs/" + jobID + "/preview/0")
	if err != nil {
		t.Fatal(err)
	}
	previewPayload := decodeEnvelope(t, preview)
	if preview.StatusCode != http.StatusOK {
		t.Fatalf("preview status %d", preview.StatusCode)
	}
	if codeOf(previewPayload) != int32(apicode.JobFailed) {
		t.Fatalf("preview code %v", previewPayload["code"])
	}
}

func TestRejectsTooManyPagesAndUnreadableImages(t *testing.T) {
	ts, client := newClient(t, &fakeEditor{})
	tooMany := make([][]byte, 21)
	for i := range tooMany {
		tooMany[i] = jpegColor(color.RGBA{R: 255, A: 255}, 20, 10)
	}
	resp := postPages(t, client, ts.URL, tooMany)
	tooManyPayload := decodeEnvelope(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("too many status %d", resp.StatusCode)
	}
	if codeOf(tooManyPayload) != int32(apicode.TooManyPages) {
		t.Fatalf("too many code %v", tooManyPayload["code"])
	}

	invalid := postPages(t, client, ts.URL, [][]byte{[]byte("not-an-image")})
	payload := decodeEnvelope(t, invalid)
	if invalid.StatusCode != http.StatusOK {
		t.Fatalf("invalid status %d", invalid.StatusCode)
	}
	if codeOf(payload) != int32(apicode.InvalidImage) {
		t.Fatalf("code %#v", payload["code"])
	}
	if payload["reason"] != "cannot read image" {
		t.Fatalf("reason %#v", payload["reason"])
	}
}

func TestExpiredJobIsRemoved(t *testing.T) {
	settings := config.Settings{
		DataDir:              t.TempDir(),
		MaxPages:             20,
		RetentionHours:       24,
		MaxLongEdge:          100,
		TextInTimeoutSeconds: 5,
		MaxUploadBytes:       20 * 1024 * 1024,
	}
	ed := &fakeEditor{results: [][]byte{jpegColor(color.RGBA{G: 255, A: 255}, 16, 16)}}
	store := jobs.NewStore(settings, ed)
	tokens := auth.NewTokens("test-secret")
	token, err := tokens.Issue("test-user")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(appserver.New(store, nil, tokens))
	t.Cleanup(ts.Close)
	client := ts.Client()
	client.Transport = &bearerTransport{base: client.Transport, token: token}
	resp := postPages(t, client, ts.URL, [][]byte{jpegColor(color.RGBA{R: 255, A: 255}, 20, 10)})
	jobID := decodeData(t, resp)["id"].(string)
	if wait(t, client, ts.URL, jobID)["status"] != "succeeded" {
		t.Fatal("expected success")
	}
	metaPath := filepath.Join(settings.DataDir, "jobs", jobID, "meta.json")
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	meta["expires_at"] = "2000-01-01T00:00:00+00:00"
	updated, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	gone, err := client.Get(ts.URL + "/jobs/" + jobID)
	if err != nil {
		t.Fatal(err)
	}
	gonePayload := decodeEnvelope(t, gone)
	if gone.StatusCode != http.StatusOK {
		t.Fatalf("status %d", gone.StatusCode)
	}
	if codeOf(gonePayload) != int32(apicode.JobNotFound) {
		t.Fatalf("code %v reason %v", gonePayload["code"], gonePayload["reason"])
	}
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Fatal("meta should be deleted")
	}
}

func getBytes(t *testing.T, client *http.Client, url string) []byte {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status %d body %s", url, resp.StatusCode, data)
	}
	return data
}

func TestCropFlagIsPassedToEditor(t *testing.T) {
	ed := &fakeEditor{results: [][]byte{jpegColor(color.RGBA{G: 255, A: 255}, 16, 16)}}
	ts, client := newClient(t, ed)
	resp := postPagesCrop(t, client, ts.URL, [][]byte{jpegColor(color.RGBA{R: 255, A: 255}, 20, 10)}, "1")
	jobID := decodeData(t, resp)["id"].(string)
	body := wait(t, client, ts.URL, jobID)
	if body["status"] != "succeeded" {
		t.Fatalf("status %v", body["status"])
	}
	if body["crop"] != float64(1) {
		t.Fatalf("crop %v", body["crop"])
	}
	if len(ed.crops) != 1 || ed.crops[0] != 1 {
		t.Fatalf("editor crop %#v", ed.crops)
	}
}

func TestPDFFromUploads(t *testing.T) {
	ts, client := newClient(t, &fakeEditor{})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("pages", "page.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(jpegColor(color.RGBA{R: 255, A: 255}, 20, 10)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/pdf", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, data)
	}
	if resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("content type %s", resp.Header.Get("Content-Type"))
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("not a pdf")
	}
}

func TestCreateJobUnauthorized(t *testing.T) {
	settings := config.Settings{
		DataDir:              t.TempDir(),
		MaxPages:             20,
		RetentionHours:       24,
		MaxLongEdge:          100,
		TextInTimeoutSeconds: 5,
		MaxUploadBytes:       20 * 1024 * 1024,
	}
	store := jobs.NewStore(settings, &fakeEditor{results: [][]byte{jpegColor(color.RGBA{G: 255, A: 255}, 8, 8)}})
	ts := httptest.NewServer(appserver.New(store, nil, auth.NewTokens("test-secret")))
	t.Cleanup(ts.Close)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("pages", "page.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(jpegColor(color.RGBA{G: 255, A: 255}, 8, 8)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer expired-token")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeEnvelope(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if codeOf(payload) != int32(apicode.Unauthorized) {
		t.Fatalf("code %v reason %v", payload["code"], payload["reason"])
	}
}

func TestCreateJobRejectsInsufficientCredits(t *testing.T) {
	wallet := &memoryWallet{remaining: 1}
	ed := &fakeEditor{results: [][]byte{
		jpegColor(color.RGBA{G: 255, A: 255}, 16, 16),
		jpegColor(color.RGBA{B: 255, A: 255}, 16, 16),
	}}
	ts, client := newClientWithWallet(t, ed, wallet)
	page := jpegColor(color.RGBA{R: 255, A: 255}, 20, 10)
	resp := postPages(t, client, ts.URL, [][]byte{page, page})
	payload := decodeEnvelope(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %#v", resp.StatusCode, payload)
	}
	if codeOf(payload) != int32(apicode.InsufficientCredits) {
		t.Fatalf("code %v reason %v", payload["code"], payload["reason"])
	}
	if wallet.remainingNow() != 1 {
		t.Fatalf("remaining %d", wallet.remainingNow())
	}
}

func TestCreateJobRefundsOnFailure(t *testing.T) {
	wallet := &memoryWallet{remaining: 2}
	ts, client := newClientWithWallet(t, &fakeEditor{err: editor.Fail("model timed out")}, wallet)
	resp := postPages(t, client, ts.URL, [][]byte{jpegColor(color.RGBA{R: 255, A: 255}, 20, 10)})
	created := decodeData(t, resp)
	if remainingOf(created) != 1 {
		t.Fatalf("create remaining %d", remainingOf(created))
	}
	jobID := created["id"].(string)
	done := wait(t, client, ts.URL, jobID)
	if done["status"] != "failed" {
		t.Fatal("expected failed")
	}
	if remainingOf(done) != 2 {
		t.Fatalf("failed remaining %d", remainingOf(done))
	}
	if wallet.remainingNow() != 2 {
		t.Fatalf("remaining %d", wallet.remainingNow())
	}
}

func TestCreateJobChargesOnSuccess(t *testing.T) {
	wallet := &memoryWallet{remaining: 2}
	ed := &fakeEditor{results: [][]byte{jpegColor(color.RGBA{G: 255, A: 255}, 16, 16)}}
	ts, client := newClientWithWallet(t, ed, wallet)
	resp := postPages(t, client, ts.URL, [][]byte{jpegColor(color.RGBA{R: 255, A: 255}, 20, 10)})
	created := decodeData(t, resp)
	if remainingOf(created) != 1 {
		t.Fatalf("create remaining %d", remainingOf(created))
	}
	jobID := created["id"].(string)
	done := wait(t, client, ts.URL, jobID)
	if done["status"] != "succeeded" {
		t.Fatal("expected success")
	}
	if remainingOf(done) != 1 {
		t.Fatalf("success remaining %d", remainingOf(done))
	}
	if wallet.remainingNow() != 1 {
		t.Fatalf("remaining %d", wallet.remainingNow())
	}
}
