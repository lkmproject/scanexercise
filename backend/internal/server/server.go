package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"scanexercise/internal/jobs"
)

type Server struct {
	store *jobs.Store
}

func New(store *jobs.Store) http.Handler {
	s := &Server{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", s.createJob)
	mux.HandleFunc("POST /pdf", s.createPDF)
	mux.HandleFunc("GET /jobs/{id}", s.getJob)
	mux.HandleFunc("GET /jobs/{id}/preview", s.listPreviews)
	mux.HandleFunc("GET /jobs/{id}/preview/{index}", s.getPreview)
	mux.HandleFunc("GET /jobs/{id}/pdf", s.getPDF)
	return mux
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	uploads, ok := readUploadedPages(w, r)
	if !ok {
		return
	}
	job, err := s.store.Create(uploads, parseCrop(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	go s.store.Run(job.ID)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"id":     job.ID,
		"status": job.Status,
	})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.loadJob(w, r.PathValue("id"))
	if !ok {
		return
	}
	body := map[string]any{
		"id":         job.ID,
		"status":     job.Status,
		"page_count": job.PageCount,
		"crop":       job.Crop,
		"error":      job.Error,
	}
	if job.Status == "succeeded" {
		previews := make([]string, job.PageCount)
		for index := 0; index < job.PageCount; index++ {
			previews[index] = "/jobs/" + job.ID + "/preview/" + strconv.Itoa(index)
		}
		body["previews"] = previews
		body["pdf"] = "/jobs/" + job.ID + "/pdf"
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) listPreviews(w http.ResponseWriter, r *http.Request) {
	job, ok := s.readyJob(w, r.PathValue("id"))
	if !ok {
		return
	}
	pages := make([]map[string]any, job.PageCount)
	for index := 0; index < job.PageCount; index++ {
		pages[index] = map[string]any{
			"index": index,
			"url":   "/jobs/" + job.ID + "/preview/" + strconv.Itoa(index),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": pages})
}

func (s *Server) getPreview(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		writeError(w, http.StatusNotFound, "找不到這一頁")
		return
	}
	data, err := s.store.Preview(r.PathValue("id"), index)
	if err != nil {
		writeJobError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) createPDF(w http.ResponseWriter, r *http.Request) {
	uploads, ok := readUploadedPages(w, r)
	if !ok {
		return
	}
	data, err := s.store.PDFFromUploads(uploads)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="exam.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) getPDF(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	data, err := s.store.PDF(jobID)
	if err != nil {
		writeJobError(w, err)
		return
	}
	s.store.Delete(jobID)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="exam.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) loadJob(w http.ResponseWriter, jobID string) (*jobs.Job, bool) {
	job, err := s.store.Get(jobID)
	if err != nil || job == nil {
		writeError(w, http.StatusNotFound, "工作不存在或已過期")
		return nil, false
	}
	return job, true
}

func (s *Server) readyJob(w http.ResponseWriter, jobID string) (*jobs.Job, bool) {
	job, ok := s.loadJob(w, jobID)
	if !ok {
		return nil, false
	}
	if job.Status == "failed" {
		message := "模型呼叫失敗"
		if job.Error != nil {
			message = *job.Error
		}
		writeError(w, http.StatusConflict, message)
		return nil, false
	}
	if job.Status != "succeeded" {
		writeError(w, http.StatusConflict, "結果尚未完成")
		return nil, false
	}
	return job, true
}

func writeJobError(w http.ResponseWriter, err error) {
	message := err.Error()
	status := http.StatusConflict
	var jobErr *jobs.Error
	if errors.As(err, &jobErr) {
		message = jobErr.Message
	}
	if message == "工作不存在或已過期" || message == "找不到這一頁" {
		status = http.StatusNotFound
	}
	writeError(w, status, message)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func readUploadedPages(w http.ResponseWriter, r *http.Request) ([][]byte, bool) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "沒有上傳頁面")
		return nil, false
	}
	files := r.MultipartForm.File["pages"]
	uploads := make([][]byte, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, "無法讀取圖片")
			return nil, false
		}
		data, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			writeError(w, http.StatusBadRequest, "無法讀取圖片")
			return nil, false
		}
		uploads = append(uploads, data)
	}
	return uploads, true
}

func parseCrop(r *http.Request) int {
	raw := r.FormValue("crop")
	if raw == "" {
		raw = r.URL.Query().Get("crop")
	}
	switch raw {
	case "1", "true", "on":
		return 1
	default:
		return 0
	}
}
