package jobs

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
	"time"

	"scanexercise/internal/apicode"
	"scanexercise/internal/config"
	"scanexercise/internal/editor"
)

type stubEditor struct{}

func (stubEditor) Edit([]byte, editor.Options) ([]byte, error) {
	return nil, nil
}

type memObjects struct {
	*diskStore
	blobs map[string][]byte
}

func (m *memObjects) presignPuts(_ context.Context, keys []string, _ time.Duration) ([]string, error) {
	urls := make([]string, len(keys))
	for i, key := range keys {
		urls[i] = "https://example.test/" + key
	}
	return urls, nil
}

func (m *memObjects) headKey(_ context.Context, key string) error {
	if _, ok := m.blobs[key]; !ok {
		return os.ErrNotExist
	}
	return nil
}

func (m *memObjects) readKey(_ context.Context, key string) ([]byte, error) {
	data, ok := m.blobs[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (m *memObjects) deleteKey(_ context.Context, key string) error {
	delete(m.blobs, key)
	return nil
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

func TestValidUploadKey(t *testing.T) {
	if !validUploadKey("u1", "uploads/u1/batch/0.jpg") {
		t.Fatal("want valid")
	}
	rejected := []string{
		"uploads/other/batch/0.jpg",
		"jobs/abc/input/0.jpg",
		"uploads/u1/../secret.jpg",
		"uploads/u1/batch/0.png",
		"uploads/u1/",
		"",
	}
	for _, key := range rejected {
		if validUploadKey("u1", key) {
			t.Fatalf("accepted %s", key)
		}
	}
}

func TestCreateUploadSlotsOnDisk(t *testing.T) {
	store := NewStore(config.Settings{DataDir: t.TempDir(), MaxPages: 10}, stubEditor{})
	_, err := store.CreateUploadSlots(context.Background(), "user", 1)
	var jobErr *Error
	if !errors.As(err, &jobErr) || jobErr.Code != apicode.InvalidRequest {
		t.Fatalf("got %v", err)
	}
}

type stubWallet struct {
	remaining int
}

func (w stubWallet) Reserve(context.Context, string, int) error { return nil }
func (w stubWallet) Refund(context.Context, string, int) error  { return nil }
func (w stubWallet) Balance(context.Context, string) (int, error) {
	return w.remaining, nil
}

func TestCreateUploadSlotsRejectsInsufficientCredits(t *testing.T) {
	settings := config.Settings{DataDir: t.TempDir(), MaxPages: 10, MaxLongEdge: 100, MaxUploadBytes: 20 << 20, RetentionHours: 24}
	store := NewStore(settings, stubEditor{})
	store.files = &memObjects{diskStore: store.files.(*diskStore), blobs: map[string][]byte{}}
	store.SetWallet(stubWallet{remaining: 1})
	_, err := store.CreateUploadSlots(context.Background(), "user-1", 2)
	assertCode(t, err, apicode.InsufficientCredits)
	slots, err := store.CreateUploadSlots(context.Background(), "user-1", 1)
	if err != nil || len(slots) != 1 {
		t.Fatalf("slots %#v err %v", slots, err)
	}
}

func TestCreateUploadSlotsRejectsCount(t *testing.T) {
	store := NewStore(config.Settings{DataDir: t.TempDir(), MaxPages: 10}, stubEditor{})
	_, err := store.CreateUploadSlots(context.Background(), "user", 0)
	assertCode(t, err, apicode.NoPages)
	_, err = store.CreateUploadSlots(context.Background(), "user", 11)
	assertCode(t, err, apicode.TooManyPages)
}

func TestCreateFromKeys(t *testing.T) {
	settings := config.Settings{
		DataDir:        t.TempDir(),
		MaxPages:       10,
		MaxLongEdge:    100,
		MaxUploadBytes: 20 << 20,
		RetentionHours: 24,
	}
	store := NewStore(settings, stubEditor{})
	objects := &memObjects{diskStore: store.files.(*diskStore), blobs: map[string][]byte{}}
	store.files = objects

	slots, err := store.CreateUploadSlots(context.Background(), "user-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 || slots[0].Key == "" || slots[0].URL == "" {
		t.Fatalf("slots %#v", slots)
	}
	objects.blobs[slots[0].Key] = jpegColor(color.RGBA{R: 255, A: 255}, 12, 8)
	objects.blobs[slots[1].Key] = jpegColor(color.RGBA{B: 255, A: 255}, 12, 8)

	job, err := store.CreateFromKeys(context.Background(), []string{slots[0].Key, slots[1].Key}, 0, 0, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if job.PageCount != 2 || job.UserID != "user-1" {
		t.Fatalf("job %#v", job)
	}
	if _, ok := objects.blobs[slots[0].Key]; ok {
		t.Fatal("upload key should be deleted")
	}
	got, err := store.files.readBlob(context.Background(), job.ID, "input/0.jpg")
	if err != nil || len(got) == 0 {
		t.Fatalf("missing input %v", err)
	}
}

func TestCreateFromKeysRejectsMissingUpload(t *testing.T) {
	settings := config.Settings{DataDir: t.TempDir(), MaxPages: 10, MaxLongEdge: 100, MaxUploadBytes: 20 << 20, RetentionHours: 24}
	store := NewStore(settings, stubEditor{})
	objects := &memObjects{diskStore: store.files.(*diskStore), blobs: map[string][]byte{}}
	store.files = objects
	slots, err := store.CreateUploadSlots(context.Background(), "user-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	objects.blobs[slots[0].Key] = jpegColor(color.RGBA{R: 255, A: 255}, 12, 8)
	_, err = store.CreateFromKeys(context.Background(), []string{slots[0].Key, slots[1].Key}, 0, 0, "user-1")
	assertCode(t, err, apicode.InvalidImage)
}

func TestCreateFromKeysRejectsForeignKey(t *testing.T) {
	settings := config.Settings{DataDir: t.TempDir(), MaxPages: 10, MaxLongEdge: 100, MaxUploadBytes: 20 << 20, RetentionHours: 24}
	store := NewStore(settings, stubEditor{})
	objects := &memObjects{diskStore: store.files.(*diskStore), blobs: map[string][]byte{}}
	store.files = objects
	_, err := store.CreateFromKeys(context.Background(), []string{"uploads/other/batch/0.jpg"}, 0, 0, "user-1")
	assertCode(t, err, apicode.InvalidRequest)
}

func TestGetReclaimsStaleRunning(t *testing.T) {
	store := NewStore(config.Settings{
		DataDir:              t.TempDir(),
		MaxPages:             10,
		MaxLongEdge:          100,
		MaxUploadBytes:       20 << 20,
		RetentionHours:       24,
		TextInTimeoutSeconds: 180,
	}, stubEditor{})
	job, err := store.Create([][]byte{jpegColor(color.White, 8, 8)}, 0, 0, "user")
	if err != nil {
		t.Fatal(err)
	}
	job.Status = "running"
	job.UpdatedAt = time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano)
	if err := store.write(job); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(job.ID)
	if err != nil || got == nil {
		t.Fatalf("Get: %v %#v", err, got)
	}
	if got.Status != "failed" || got.Error == nil || *got.Error != "model timed out" {
		t.Fatalf("got %#v", got)
	}
}

func TestGetKeepsFreshRunning(t *testing.T) {
	store := NewStore(config.Settings{
		DataDir:              t.TempDir(),
		MaxPages:             10,
		MaxLongEdge:          100,
		MaxUploadBytes:       20 << 20,
		RetentionHours:       24,
		TextInTimeoutSeconds: 180,
	}, stubEditor{})
	job, err := store.Create([][]byte{jpegColor(color.White, 8, 8)}, 0, 0, "user")
	if err != nil {
		t.Fatal(err)
	}
	job.Status = "running"
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.write(job); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(job.ID)
	if err != nil || got == nil {
		t.Fatalf("Get: %v %#v", err, got)
	}
	if got.Status != "running" {
		t.Fatalf("status %s", got.Status)
	}
}

func assertCode(t *testing.T, err error, code apicode.Code) {
	t.Helper()
	var jobErr *Error
	if !errors.As(err, &jobErr) || jobErr.Code != code {
		t.Fatalf("got %v want %v", err, code)
	}
}
