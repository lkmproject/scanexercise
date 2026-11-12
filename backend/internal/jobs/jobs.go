package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"scanexercise/internal/apicode"
	"scanexercise/internal/config"
	"scanexercise/internal/editor"
	"scanexercise/internal/preprocess"
)

var jobIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type Error struct {
	Code    apicode.Code
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func fail(code apicode.Code, message string) error {
	if message == "" {
		message = code.Reason()
	}
	return &Error{Code: code, Message: message}
}

type Job struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id,omitempty"`
	Status       string   `json:"status"`
	PageCount    int      `json:"page_count"`
	Crop         int      `json:"crop"`
	Binarization int      `json:"binarization"`
	Charged      int      `json:"charged,omitempty"`
	Completed    int      `json:"completed,omitempty"`
	Refunded     bool     `json:"refunded,omitempty"`
	Error        *string  `json:"error"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
	ExpiresAt    string   `json:"expires_at"`
	UploadKeys   []string `json:"upload_keys,omitempty"`
}

type Wallet interface {
	Reserve(ctx context.Context, userID string, pages int) error
	Refund(ctx context.Context, userID string, pages int) error
	Balance(ctx context.Context, userID string) (int, error)
}

type fileStore interface {
	writeMeta(ctx context.Context, job *Job) error
	readMeta(ctx context.Context, jobID string) (*Job, error)
	writeBlob(ctx context.Context, jobID, rel string, data []byte) error
	readBlob(ctx context.Context, jobID, rel string) ([]byte, error)
	remove(ctx context.Context, jobID string) error
	removeBlobs(ctx context.Context, jobID string, rels ...string) error
	listIDs(ctx context.Context) []string
}

type UploadSlot struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

type objectStore interface {
	presignPuts(ctx context.Context, keys []string, expire time.Duration) ([]string, error)
	headKey(ctx context.Context, key string) error
	readKey(ctx context.Context, key string) ([]byte, error)
	deleteKey(ctx context.Context, key string) error
}

type Store struct {
	settings   config.Settings
	editor     editor.Editor
	wallet     Wallet
	files      fileStore
	asyncStart func(jobID string) error
}

func NewStore(settings config.Settings, ed editor.Editor) *Store {
	return &Store{
		settings: settings,
		editor:   ed,
		files:    newDiskStore(filepath.Join(settings.DataDir, "jobs")),
	}
}

func NewAWSStore(settings config.Settings, ed editor.Editor, cfg aws.Config) *Store {
	return &Store{
		settings: settings,
		editor:   ed,
		files:    newAWSStore(cfg),
	}
}

func (s *Store) SetWallet(wallet Wallet) {
	s.wallet = wallet
}

func (s *Store) SetAsyncStart(start func(jobID string) error) {
	s.asyncStart = start
}

func (s *Store) Create(uploads [][]byte, crop, binarization int, userID string) (*Job, error) {
	s.Cleanup()
	prepared, err := s.prepareUploads(uploads)
	if err != nil {
		return nil, err
	}
	return s.finishCreate(context.Background(), prepared, crop, binarization, userID)
}

func (s *Store) CreateUploadSlots(ctx context.Context, userID string, count int) ([]UploadSlot, error) {
	if count <= 0 {
		return nil, fail(apicode.NoPages, "")
	}
	if count > s.settings.MaxPages {
		return nil, fail(apicode.TooManyPages, "at most "+strconv.Itoa(s.settings.MaxPages)+" pages")
	}
	if s.wallet != nil {
		remaining, err := s.wallet.Balance(ctx, userID)
		if err != nil {
			return nil, fail(apicode.Internal, "cannot read balance")
		}
		if remaining < count {
			return nil, fail(apicode.InsufficientCredits, "")
		}
	}
	objects, ok := s.files.(objectStore)
	if !ok {
		return nil, fail(apicode.InvalidRequest, "direct upload unavailable")
	}
	batch := hexUUID()
	owner := safeUserID(userID)
	keys := make([]string, count)
	for i := 0; i < count; i++ {
		keys[i] = "uploads/" + owner + "/" + batch + "/" + strconv.Itoa(i) + ".jpg"
	}
	urls, err := objects.presignPuts(ctx, keys, 15*time.Minute)
	if err != nil {
		return nil, fail(apicode.Internal, "cannot create upload")
	}
	slots := make([]UploadSlot, count)
	for i := range keys {
		slots[i] = UploadSlot{Key: keys[i], URL: urls[i]}
	}
	return slots, nil
}

func (s *Store) CreateFromKeys(ctx context.Context, keys []string, crop, binarization int, userID string) (*Job, error) {
	if len(keys) == 0 {
		return nil, fail(apicode.NoPages, "")
	}
	if len(keys) > s.settings.MaxPages {
		return nil, fail(apicode.TooManyPages, "at most "+strconv.Itoa(s.settings.MaxPages)+" pages")
	}
	objects, ok := s.files.(objectStore)
	if !ok {
		return nil, fail(apicode.InvalidRequest, "direct upload unavailable")
	}
	s.Cleanup()
	owner := safeUserID(userID)
	for _, key := range keys {
		if !validUploadKey(owner, key) {
			return nil, fail(apicode.InvalidRequest, "invalid upload key")
		}
		if err := objects.headKey(ctx, key); err != nil {
			return nil, fail(apicode.InvalidImage, "upload missing")
		}
	}
	job, err := s.finishCreateMeta(ctx, len(keys), crop, binarization, userID, keys)
	if err != nil {
		return nil, err
	}
	if s.asyncStart == nil {
		if err := s.ingestUploadKeys(ctx, job); err != nil {
			_ = s.files.remove(ctx, job.ID)
			return nil, err
		}
	}
	return job, nil
}

func (s *Store) prepareUploads(uploads [][]byte) ([][]byte, error) {
	if len(uploads) == 0 {
		return nil, fail(apicode.NoPages, "")
	}
	if len(uploads) > s.settings.MaxPages {
		return nil, fail(apicode.TooManyPages, "at most "+strconv.Itoa(s.settings.MaxPages)+" pages")
	}
	for _, upload := range uploads {
		if len(upload) > s.settings.MaxUploadBytes {
			return nil, fail(apicode.ImageTooLarge, "")
		}
	}
	prepared := make([][]byte, 0, len(uploads))
	for _, upload := range uploads {
		page, err := preprocess.ForTextIn(upload)
		if err != nil {
			return nil, fail(apicode.InvalidImage, err.Error())
		}
		prepared = append(prepared, page)
	}
	return prepared, nil
}

func (s *Store) finishCreate(ctx context.Context, prepared [][]byte, crop, binarization int, userID string) (*Job, error) {
	job, err := s.finishCreateMeta(ctx, len(prepared), crop, binarization, userID, nil)
	if err != nil {
		return nil, err
	}
	for index, page := range prepared {
		if err := s.files.writeBlob(ctx, job.ID, "input/"+strconv.Itoa(index)+".jpg", page); err != nil {
			_ = s.files.remove(ctx, job.ID)
			return nil, fail(apicode.Internal, "cannot create job")
		}
	}
	return job, nil
}

func (s *Store) finishCreateMeta(ctx context.Context, pageCount, crop, binarization int, userID string, uploadKeys []string) (*Job, error) {
	jobID := hexUUID()
	if crop != 1 {
		crop = 0
	}
	if binarization != 1 {
		binarization = 0
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	job := &Job{
		ID:           jobID,
		UserID:       userID,
		Status:       "pending",
		PageCount:    pageCount,
		Crop:         crop,
		Binarization: binarization,
		Error:        nil,
		CreatedAt:    stamp,
		UpdatedAt:    stamp,
		ExpiresAt:    now.Add(time.Duration(s.settings.RetentionHours) * time.Hour).Format(time.RFC3339Nano),
		UploadKeys:   append([]string(nil), uploadKeys...),
	}
	if err := s.write(job); err != nil {
		_ = s.files.remove(ctx, jobID)
		return nil, fail(apicode.Internal, "cannot create job")
	}
	return job, nil
}

func (s *Store) ingestUploadKeys(ctx context.Context, job *Job) error {
	objects, ok := s.files.(objectStore)
	if !ok {
		return fail(apicode.InvalidRequest, "direct upload unavailable")
	}
	if job == nil || len(job.UploadKeys) == 0 {
		return nil
	}
	keys := append([]string(nil), job.UploadKeys...)
	for index, key := range keys {
		if err := ctx.Err(); err != nil {
			return fail(apicode.Internal, "model timed out")
		}
		log.Printf("ingest job=%s page=%d key=%s", job.ID, index, key)
		s.touch(job)
		raw, err := objects.readKey(ctx, key)
		if err != nil {
			return fail(apicode.InvalidImage, "upload missing")
		}
		prepared, err := s.prepareUploads([][]byte{raw})
		if err != nil {
			return err
		}
		if err := s.files.writeBlob(ctx, job.ID, "input/"+strconv.Itoa(index)+".jpg", prepared[0]); err != nil {
			return fail(apicode.Internal, "cannot create job")
		}
	}
	for _, key := range keys {
		_ = objects.deleteKey(ctx, key)
	}
	job.UploadKeys = nil
	job.PageCount = len(keys)
	return s.write(job)
}

func validUploadKey(userID, key string) bool {
	prefix := "uploads/" + userID + "/"
	if userID == "" || !strings.HasPrefix(key, prefix) {
		return false
	}
	rest := strings.TrimPrefix(key, prefix)
	if rest == "" || strings.Contains(rest, "..") || strings.HasPrefix(rest, "/") {
		return false
	}
	return strings.HasSuffix(rest, ".jpg")
}

func safeUserID(userID string) string {
	var b strings.Builder
	for _, r := range userID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func (s *Store) Charge(ctx context.Context, job *Job) error {
	if s.wallet == nil || job == nil || job.PageCount <= 0 {
		return nil
	}
	if err := s.wallet.Reserve(ctx, job.UserID, job.PageCount); err != nil {
		return err
	}
	job.Charged = job.PageCount
	if err := s.write(job); err != nil {
		if refundErr := s.wallet.Refund(ctx, job.UserID, job.PageCount); refundErr != nil {
			log.Printf("refund after charge write failed job=%s: %v", job.ID, refundErr)
		}
		job.Charged = 0
		return err
	}
	return nil
}

func (s *Store) Remaining(ctx context.Context, userID string) int {
	if s.wallet == nil || userID == "" {
		return 0
	}
	remaining, err := s.wallet.Balance(ctx, userID)
	if err != nil {
		log.Printf("balance user=%s: %v", userID, err)
		return 0
	}
	return remaining
}

func (s *Store) Start(jobID string) {
	if s.asyncStart != nil {
		if err := s.asyncStart(jobID); err != nil {
			log.Printf("async start job=%s: %v", jobID, err)
			job, _ := s.read(jobID)
			if job != nil {
				s.markFailed(job, "model call failed")
			}
		}
		return
	}
	go s.Run(jobID)
}

func (s *Store) Run(jobID string) {
	s.RunContext(context.Background(), jobID)
}

func (s *Store) RunContext(ctx context.Context, jobID string) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("run panic job=%s: %v", jobID, rec)
			job, _ := s.read(jobID)
			if job != nil && (job.Status == "pending" || job.Status == "running") {
				s.markFailed(job, "model call failed")
			}
		}
	}()
	job, err := s.read(jobID)
	if err != nil || job == nil {
		log.Printf("run missing job=%s err=%v", jobID, err)
		return
	}
	if job.Status != "pending" {
		log.Printf("run skip job=%s status=%s", jobID, job.Status)
		return
	}
	log.Printf("run start job=%s pages=%d keys=%d", jobID, job.PageCount, len(job.UploadKeys))
	job.Status = "running"
	s.touch(job)
	if len(job.UploadKeys) > 0 {
		if err := s.ingestUploadKeys(ctx, job); err != nil {
			message := "cannot read image"
			var jobErr *Error
			if errors.As(err, &jobErr) && jobErr.Message != "" {
				message = jobErr.Message
			}
			s.markFailed(job, message)
			return
		}
	}
	s.erasePages(ctx, job)
}

const textInConcurrency = 2

func (s *Store) erasePages(ctx context.Context, job *Job) {
	var mu sync.Mutex
	opts := editor.Options{Crop: job.Crop, Binarization: job.Binarization}
	stopped := false
	reason := ""
	succeeded := 0

	for start := 0; start < job.PageCount; start += textInConcurrency {
		if ctx.Err() != nil {
			job.Completed = succeeded
			s.markFailed(job, "model timed out")
			return
		}
		mu.Lock()
		halt := stopped
		open := !halt && s.touch(job) && job.Status == "running"
		mu.Unlock()
		if halt {
			break
		}
		if !open {
			log.Printf("run abort job=%s status=%s", job.ID, job.Status)
			return
		}
		end := start + textInConcurrency
		if end > job.PageCount {
			end = job.PageCount
		}
		var wg sync.WaitGroup
		for index := start; index < end; index++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				message, ok := s.erasePage(ctx, job.ID, index, opts)
				mu.Lock()
				defer mu.Unlock()
				if !ok {
					stopped = true
					if reason == "" {
						reason = message
					}
					return
				}
				succeeded++
				job.Completed = succeeded
				if !s.touch(job) {
					stopped = true
				}
			}(index)
		}
		wg.Wait()
		mu.Lock()
		halt = stopped
		mu.Unlock()
		if halt {
			break
		}
	}

	mu.Lock()
	job.Completed = succeeded
	halt := stopped
	message := reason
	mu.Unlock()
	if halt {
		if message == "" {
			message = "model call failed"
		}
		s.markFailed(job, message)
		return
	}
	if !s.syncOpen(job) {
		log.Printf("run abort job=%s status=%s", job.ID, job.Status)
		return
	}
	job.Status = "succeeded"
	job.Error = nil
	_ = s.write(job)
	log.Printf("run done job=%s pages=%d", job.ID, job.PageCount)
}

func (s *Store) erasePage(ctx context.Context, jobID string, index int, opts editor.Options) (string, bool) {
	if err := ctx.Err(); err != nil {
		return "model timed out", false
	}
	source, err := s.files.readBlob(ctx, jobID, "input/"+strconv.Itoa(index)+".jpg")
	if err != nil {
		return "model call failed", false
	}
	edited, err := s.editor.Edit(source, opts)
	if err != nil {
		log.Printf("textin page job=%s index=%d err=%v", jobID, index, err)
		message := "model call failed"
		var edErr *editor.Error
		if errors.As(err, &edErr) && edErr.Message != "" {
			message = edErr.Message
		}
		return message, false
	}
	if err := s.files.writeBlob(ctx, jobID, "output/"+strconv.Itoa(index)+".jpg", edited); err != nil {
		return "model call failed", false
	}
	return "", true
}

func (s *Store) OutputIndexes(job *Job) []int {
	if job == nil || (job.Status != "succeeded" && job.Status != "failed") {
		return nil
	}
	indexes := make([]int, 0, job.PageCount)
	for index := 0; index < job.PageCount; index++ {
		if _, err := s.files.readBlob(context.Background(), job.ID, "output/"+strconv.Itoa(index)+".jpg"); err == nil {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func (s *Store) Get(jobID string) (*Job, error) {
	if err := s.deleteIfExpired(jobID); err != nil {
		var jobErr *Error
		if errors.As(err, &jobErr) {
			return nil, nil
		}
		return nil, err
	}
	job, err := s.read(jobID)
	if err != nil || job == nil {
		return job, err
	}
	s.reclaimStale(job)
	return job, nil
}

func (s *Store) touch(job *Job) bool {
	if job == nil || !s.syncOpen(job) {
		return false
	}
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return s.write(job) == nil
}

func (s *Store) syncOpen(job *Job) bool {
	if job == nil {
		return false
	}
	latest, err := s.read(job.ID)
	if err != nil || latest == nil {
		return false
	}
	job.Charged = latest.Charged
	job.Refunded = latest.Refunded
	if latest.Completed > job.Completed {
		job.Completed = latest.Completed
	}
	if latest.Status != "pending" && latest.Status != "running" {
		job.Status = latest.Status
		job.Error = latest.Error
		return false
	}
	return true
}

func (s *Store) reclaimStale(job *Job) {
	if job == nil || (job.Status != "pending" && job.Status != "running") {
		return
	}
	stamp := job.UpdatedAt
	if stamp == "" {
		stamp = job.CreatedAt
	}
	updated, err := parseTime(stamp)
	if err != nil {
		return
	}
	limit := 13 * time.Minute
	if job.Status == "running" {
		limit = time.Duration(s.settings.TextInTimeoutSeconds+60) * time.Second
		if limit < 4*time.Minute {
			limit = 4 * time.Minute
		}
	}
	if time.Since(updated.UTC()) < limit {
		return
	}
	log.Printf("stale job=%s status=%s age=%s", job.ID, job.Status, time.Since(updated.UTC()).Truncate(time.Second))
	s.markFailed(job, "model timed out")
}

func (s *Store) Preview(jobID string, index int) ([]byte, error) {
	job, err := s.Get(jobID)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case "succeeded", "failed":
	default:
		return nil, fail(apicode.JobNotReady, "")
	}
	if index < 0 || index >= job.PageCount {
		return nil, fail(apicode.PageNotFound, "")
	}
	data, err := s.files.readBlob(context.Background(), jobID, "output/"+strconv.Itoa(index)+".jpg")
	if err != nil {
		return nil, fail(apicode.PageNotFound, "")
	}
	return data, nil
}

func (s *Store) Delete(jobID string) {
	_ = s.files.remove(context.Background(), jobID)
}

func (s *Store) Cleanup() {
	for _, id := range s.files.listIDs(context.Background()) {
		_ = s.deleteIfExpired(id)
	}
}

func (s *Store) markFailed(job *Job, message string) {
	if job == nil || !s.syncOpen(job) {
		return
	}
	if job.Completed <= 0 {
		_ = s.files.removeBlobs(context.Background(), job.ID, "output/")
	}
	job.Status = "failed"
	job.Error = &message
	s.refund(job)
	_ = s.write(job)
}

func (s *Store) refund(job *Job) {
	if s.wallet == nil || job == nil || job.Refunded || job.UserID == "" {
		return
	}
	pages := job.Charged - job.Completed
	if pages <= 0 {
		if job.Charged > 0 {
			job.Refunded = true
		}
		return
	}
	if err := s.wallet.Refund(context.Background(), job.UserID, pages); err != nil {
		log.Printf("refund job=%s: %v", job.ID, err)
		return
	}
	job.Refunded = true
}

func RequireReady(job *Job) error {
	if job == nil {
		return fail(apicode.JobNotFound, "")
	}
	if job.Status == "failed" {
		message := apicode.JobFailed.Reason()
		if job.Error != nil {
			message = *job.Error
		}
		return fail(apicode.JobFailed, message)
	}
	if job.Status != "succeeded" {
		return fail(apicode.JobNotReady, "")
	}
	return nil
}

func (s *Store) deleteIfExpired(jobID string) error {
	job, err := s.read(jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}
	expiresAt, err := parseTime(job.ExpiresAt)
	if err != nil {
		return nil
	}
	if !expiresAt.After(time.Now().UTC()) {
		if job.Status != "succeeded" {
			s.refund(job)
		}
		s.Delete(jobID)
	}
	return nil
}

func (s *Store) write(job *Job) error {
	if !jobIDPattern.MatchString(job.ID) {
		return fail(apicode.JobNotFound, "")
	}
	return s.files.writeMeta(context.Background(), job)
}

func (s *Store) read(jobID string) (*Job, error) {
	if !jobIDPattern.MatchString(jobID) {
		return nil, fail(apicode.JobNotFound, "")
	}
	return s.files.readMeta(context.Background(), jobID)
}

func hexUUID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw)
}

func parseTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}
