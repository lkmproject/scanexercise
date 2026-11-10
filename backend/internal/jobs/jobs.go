package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"scanexercise/internal/apicode"
	"scanexercise/internal/config"
	"scanexercise/internal/editor"
	"scanexercise/internal/pdf"
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
	ID        string  `json:"id"`
	UserID    string  `json:"user_id,omitempty"`
	Status    string  `json:"status"`
	PageCount int     `json:"page_count"`
	Crop      int     `json:"crop"`
	Charged   int     `json:"charged,omitempty"`
	Refunded  bool    `json:"refunded,omitempty"`
	Error     *string `json:"error"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt string  `json:"expires_at"`
}

type Wallet interface {
	Reserve(ctx context.Context, userID string, pages int) error
	Refund(ctx context.Context, userID string, pages int) error
	Balance(ctx context.Context, userID string) (int, error)
}

type Store struct {
	settings config.Settings
	editor   editor.Editor
	wallet   Wallet
	root     string
	mu       sync.Mutex
}

func NewStore(settings config.Settings, ed editor.Editor) *Store {
	root := filepath.Join(settings.DataDir, "jobs")
	_ = os.MkdirAll(root, 0o755)
	return &Store{settings: settings, editor: ed, root: root}
}

func (s *Store) SetWallet(wallet Wallet) {
	s.wallet = wallet
}

func (s *Store) Create(uploads [][]byte, crop int, userID string) (*Job, error) {
	s.Cleanup()
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
		page, err := preprocess.PreparePage(upload, s.settings.MaxLongEdge)
		if err != nil {
			return nil, fail(apicode.InvalidImage, err.Error())
		}
		prepared = append(prepared, page)
	}
	jobID := hexUUID()
	directory := s.jobDir(jobID)
	if err := os.MkdirAll(filepath.Join(directory, "input"), 0o755); err != nil {
		return nil, fail(apicode.Internal, "cannot create job")
	}
	for index, page := range prepared {
		path := filepath.Join(directory, "input", strconv.Itoa(index)+".jpg")
		if err := os.WriteFile(path, page, 0o644); err != nil {
			return nil, fail(apicode.Internal, "cannot create job")
		}
	}
	if crop != 1 {
		crop = 0
	}
	now := time.Now().UTC()
	job := &Job{
		ID:        jobID,
		UserID:    userID,
		Status:    "pending",
		PageCount: len(prepared),
		Crop:      crop,
		Error:     nil,
		CreatedAt: now.Format(time.RFC3339Nano),
		ExpiresAt: now.Add(time.Duration(s.settings.RetentionHours) * time.Hour).Format(time.RFC3339Nano),
	}
	if err := s.write(job); err != nil {
		return nil, fail(apicode.Internal, "cannot create job")
	}
	return job, nil
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

func (s *Store) PDFFromUploads(uploads [][]byte) ([]byte, error) {
	if len(uploads) == 0 {
		return nil, fail(apicode.NoPages, "")
	}
	if len(uploads) > s.settings.MaxPages {
		return nil, fail(apicode.TooManyPages, "at most "+strconv.Itoa(s.settings.MaxPages)+" pages")
	}
	prepared := make([][]byte, 0, len(uploads))
	for _, upload := range uploads {
		if len(upload) > s.settings.MaxUploadBytes {
			return nil, fail(apicode.ImageTooLarge, "")
		}
		page, err := preprocess.PreparePage(upload, s.settings.MaxLongEdge)
		if err != nil {
			return nil, fail(apicode.InvalidImage, err.Error())
		}
		prepared = append(prepared, page)
	}
	data, err := pdf.Build(prepared)
	if err != nil {
		return nil, fail(apicode.PDFFailed, "")
	}
	return data, nil
}

func (s *Store) Run(jobID string) {
	job, err := s.read(jobID)
	if err != nil || job == nil || job.Status != "pending" {
		return
	}
	job.Status = "running"
	_ = s.write(job)
	directory := s.jobDir(jobID)
	outputDir := filepath.Join(directory, "output")
	_ = os.MkdirAll(outputDir, 0o755)
	pages := make([][]byte, 0, job.PageCount)
	for index := 0; index < job.PageCount; index++ {
		source, err := os.ReadFile(filepath.Join(directory, "input", strconv.Itoa(index)+".jpg"))
		if err != nil {
			s.markFailed(job, "model call failed")
			return
		}
		edited, err := s.editor.Edit(source, editor.Options{Crop: job.Crop})
		if err != nil {
			message := "model call failed"
			var edErr *editor.Error
			if errors.As(err, &edErr) {
				message = edErr.Message
			}
			s.markFailed(job, message)
			return
		}
		pages = append(pages, edited)
	}
	for index, page := range pages {
		path := filepath.Join(outputDir, strconv.Itoa(index)+".jpg")
		if err := os.WriteFile(path, page, 0o644); err != nil {
			s.markFailed(job, "model call failed")
			return
		}
	}
	pdfBytes, err := pdf.Build(pages)
	if err != nil {
		s.markFailed(job, "model call failed")
		return
	}
	if err := os.WriteFile(filepath.Join(directory, "result.pdf"), pdfBytes, 0o644); err != nil {
		s.markFailed(job, "model call failed")
		return
	}
	job.Status = "succeeded"
	job.Error = nil
	_ = s.write(job)
}

func (s *Store) Get(jobID string) (*Job, error) {
	if err := s.deleteIfExpired(jobID); err != nil {
		var jobErr *Error
		if errors.As(err, &jobErr) {
			return nil, nil
		}
		return nil, err
	}
	return s.read(jobID)
}

func (s *Store) Preview(jobID string, index int) ([]byte, error) {
	job, err := s.Get(jobID)
	if err != nil {
		return nil, err
	}
	if err := RequireReady(job); err != nil {
		return nil, err
	}
	if index < 0 || index >= job.PageCount {
		return nil, fail(apicode.PageNotFound, "")
	}
	data, err := os.ReadFile(filepath.Join(s.jobDir(jobID), "output", strconv.Itoa(index)+".jpg"))
	if err != nil {
		return nil, fail(apicode.PageNotFound, "")
	}
	return data, nil
}

func (s *Store) PDF(jobID string) ([]byte, error) {
	job, err := s.Get(jobID)
	if err != nil {
		return nil, err
	}
	if err := RequireReady(job); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(s.jobDir(jobID), "result.pdf"))
	if err != nil {
		return nil, fail(apicode.JobNotReady, "")
	}
	return data, nil
}

func (s *Store) Delete(jobID string) {
	_ = os.RemoveAll(s.jobDir(jobID))
}

func (s *Store) Cleanup() {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			_ = s.deleteIfExpired(entry.Name())
		}
	}
}

func (s *Store) markFailed(job *Job, message string) {
	_ = os.RemoveAll(filepath.Join(s.jobDir(job.ID), "output"))
	_ = os.Remove(filepath.Join(s.jobDir(job.ID), "result.pdf"))
	job.Status = "failed"
	job.Error = &message
	s.refund(job)
	_ = s.write(job)
}

func (s *Store) refund(job *Job) {
	if s.wallet == nil || job == nil || job.Refunded || job.Charged <= 0 || job.UserID == "" {
		return
	}
	if err := s.wallet.Refund(context.Background(), job.UserID, job.Charged); err != nil {
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

func (s *Store) jobDir(jobID string) string {
	return filepath.Join(s.root, jobID)
}

func (s *Store) write(job *Job) error {
	if !jobIDPattern.MatchString(job.ID) {
		return fail(apicode.JobNotFound, "")
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.WriteFile(filepath.Join(s.jobDir(job.ID), "meta.json"), payload, 0o644)
}

func (s *Store) read(jobID string) (*Job, error) {
	if !jobIDPattern.MatchString(jobID) {
		return nil, fail(apicode.JobNotFound, "")
	}
	path := filepath.Join(s.jobDir(jobID), "meta.json")
	s.mu.Lock()
	data, err := os.ReadFile(path)
	s.mu.Unlock()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
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
