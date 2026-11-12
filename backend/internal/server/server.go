package server

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"scanexercise/internal/apicode"
	"scanexercise/internal/auth"
	"scanexercise/internal/jobs"
)

type Server struct {
	store    *jobs.Store
	accounts *auth.Service
	tokens   *auth.Tokens
}

const APIPrefix = "/scanexercise"

type envelope struct {
	Code   int32  `json:"code"`
	Reason string `json:"reason"`
	Data   any    `json:"data"`
}

func New(store *jobs.Store, accounts *auth.Service, tokens *auth.Tokens) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{store: store, accounts: accounts, tokens: tokens}
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.MaxMultipartMemory = 32 << 20
	_ = engine.SetTrustedProxies(nil)

	root := engine.Group(APIPrefix)
	root.POST("/auth/google", s.googleAuth)

	authed := root.Group("/")
	authed.Use(s.requireAuth)
	authed.GET("/me", s.me)
	authed.POST("/uploads", s.createUploads)
	authed.POST("/jobs", s.createJob)
	authed.POST("/pdf", s.createPDF)
	authed.GET("/jobs/:id/preview/:index", s.getPreview)
	authed.GET("/jobs/:id/preview", s.listPreviews)
	authed.GET("/jobs/:id/pdf", s.getPDF)
	authed.GET("/jobs/:id", s.getJob)
	return engine
}

func (s *Server) requireAuth(c *gin.Context) {
	if s.tokens == nil {
		rejectAuth(c, apicode.AuthUnavailable)
		return
	}
	header := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		rejectAuth(c, apicode.Unauthorized)
		return
	}
	userID, err := s.tokens.Parse(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		rejectAuth(c, apicode.Unauthorized)
		return
	}
	c.Set("userID", userID)
	c.Next()
}

func rejectAuth(c *gin.Context, code apicode.Code) {
	if c.Request.Body != nil {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		_ = c.Request.Body.Close()
	}
	writeFail(c, code, "")
	c.Abort()
}

func (s *Server) googleAuth(c *gin.Context) {
	var body struct {
		IDToken string `json:"id_token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeFail(c, apicode.InvalidRequest, "unable to read login data")
		return
	}
	user, err := s.accounts.SignInGoogle(c.Request.Context(), body.IDToken)
	if err != nil {
		if errors.Is(err, auth.ErrUnavailable) {
			writeFail(c, apicode.AuthUnavailable, "")
			return
		}
		if errors.Is(err, auth.ErrInvalidToken) {
			writeFail(c, apicode.AuthInvalid, "")
			return
		}
		log.Printf("google auth: %v", err)
		writeFail(c, apicode.AuthFailed, "")
		return
	}
	if s.tokens == nil {
		writeFail(c, apicode.AuthUnavailable, "")
		return
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		log.Printf("jwt issue: %v", err)
		writeFail(c, apicode.AuthFailed, "")
		return
	}
	writeOK(c, auth.Session{Profile: *user, Token: token})
}

func (s *Server) me(c *gin.Context) {
	if s.accounts == nil {
		writeFail(c, apicode.AuthUnavailable, "")
		return
	}
	profile, err := s.accounts.Me(c.Request.Context(), c.GetString("userID"))
	if err != nil {
		if errors.Is(err, auth.ErrUnavailable) {
			writeFail(c, apicode.AuthUnavailable, "")
			return
		}
		if errors.Is(err, auth.ErrUnauthorized) {
			writeFail(c, apicode.Unauthorized, "")
			return
		}
		log.Printf("me: %v", err)
		writeFail(c, apicode.Internal, "")
		return
	}
	writeOK(c, profile)
}

func (s *Server) createUploads(c *gin.Context) {
	var body struct {
		Count int `json:"count"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeFail(c, apicode.InvalidRequest, "unable to read upload request")
		return
	}
	slots, err := s.store.CreateUploadSlots(c.Request.Context(), c.GetString("userID"), body.Count)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	writeOK(c, map[string]any{"files": slots})
}

func (s *Server) createJob(c *gin.Context) {
	if strings.Contains(c.GetHeader("Content-Type"), "application/json") {
		s.createJobFromKeys(c)
		return
	}
	uploads, ok := readUploadedPages(c)
	if !ok {
		return
	}
	job, err := s.store.Create(uploads, parseFlag(c, "crop"), parseFlag(c, "binarization"), c.GetString("userID"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	s.commitJob(c, job)
}

func (s *Server) createJobFromKeys(c *gin.Context) {
	var body struct {
		Keys         []string `json:"keys"`
		Crop         int      `json:"crop"`
		Binarization int      `json:"binarization"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeFail(c, apicode.InvalidRequest, "unable to read job data")
		return
	}
	job, err := s.store.CreateFromKeys(c.Request.Context(), body.Keys, body.Crop, body.Binarization, c.GetString("userID"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	s.commitJob(c, job)
}

func (s *Server) commitJob(c *gin.Context, job *jobs.Job) {
	if err := s.store.Charge(c.Request.Context(), job); err != nil {
		s.store.Delete(job.ID)
		if errors.Is(err, auth.ErrInsufficient) {
			writeFail(c, apicode.InsufficientCredits, "")
			return
		}
		log.Printf("charge job=%s: %v", job.ID, err)
		writeFail(c, apicode.Internal, "")
		return
	}
	log.Printf("jobs start id=%s pages=%d", job.ID, job.PageCount)
	s.store.Start(job.ID)
	writeOK(c, map[string]any{
		"id":        job.ID,
		"status":    job.Status,
		"remaining": s.store.Remaining(c.Request.Context(), job.UserID),
	})
}

func (s *Server) getJob(c *gin.Context) {
	job, ok := s.loadJob(c, c.Param("id"))
	if !ok {
		return
	}
	body := map[string]any{
		"id":           job.ID,
		"status":       job.Status,
		"page_count":   job.PageCount,
		"crop":         job.Crop,
		"binarization": job.Binarization,
		"error":        job.Error,
		"remaining":    s.store.Remaining(c.Request.Context(), job.UserID),
	}
	readyPages := 0
	if job.Status == "succeeded" {
		readyPages = job.PageCount
		body["pdf"] = "/jobs/" + job.ID + "/pdf"
	} else if job.Status == "failed" {
		readyPages = job.Completed
	}
	if readyPages > 0 {
		previews := make([]string, readyPages)
		for index := 0; index < readyPages; index++ {
			previews[index] = "/jobs/" + job.ID + "/preview/" + strconv.Itoa(index)
		}
		body["previews"] = previews
	}
	writeOK(c, body)
}

func (s *Server) listPreviews(c *gin.Context) {
	job, ok := s.readyJob(c, c.Param("id"))
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
	writeOK(c, map[string]any{"pages": pages})
}

func (s *Server) getPreview(c *gin.Context) {
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		writeFail(c, apicode.PageNotFound, "")
		return
	}
	data, err := s.store.Preview(c.Param("id"), index)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	c.Data(http.StatusOK, "image/jpeg", data)
}

func (s *Server) createPDF(c *gin.Context) {
	uploads, ok := readUploadedPages(c)
	if !ok {
		return
	}
	data, err := s.store.PDFFromUploads(uploads)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="exam.pdf"`)
	c.Data(http.StatusOK, "application/pdf", data)
}

func (s *Server) getPDF(c *gin.Context) {
	jobID := c.Param("id")
	data, err := s.store.PDF(jobID)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	s.store.Delete(jobID)
	c.Header("Content-Disposition", `attachment; filename="exam.pdf"`)
	c.Data(http.StatusOK, "application/pdf", data)
}

func (s *Server) loadJob(c *gin.Context, jobID string) (*jobs.Job, bool) {
	job, err := s.store.Get(jobID)
	if err != nil {
		writeStoreError(c, err)
		return nil, false
	}
	if job == nil {
		writeFail(c, apicode.JobNotFound, "")
		return nil, false
	}
	return job, true
}

func (s *Server) readyJob(c *gin.Context, jobID string) (*jobs.Job, bool) {
	job, ok := s.loadJob(c, jobID)
	if !ok {
		return nil, false
	}
	if err := jobs.RequireReady(job); err != nil {
		writeStoreError(c, err)
		return nil, false
	}
	return job, true
}

func writeStoreError(c *gin.Context, err error) {
	code := apicode.Internal
	reason := err.Error()
	var jobErr *jobs.Error
	if errors.As(err, &jobErr) {
		code = jobErr.Code
		reason = jobErr.Message
	}
	writeFail(c, code, reason)
}

func writeOK(c *gin.Context, data any) {
	writeEnvelope(c, apicode.OK, apicode.OK.Reason(), data)
}

func writeFail(c *gin.Context, code apicode.Code, reason string) {
	if reason == "" {
		reason = code.Reason()
	}
	writeEnvelope(c, code, reason, nil)
}

func writeEnvelope(c *gin.Context, code apicode.Code, reason string, data any) {
	c.JSON(code.HTTPStatus(), envelope{
		Code:   int32(code),
		Reason: reason,
		Data:   data,
	})
}

func readUploadedPages(c *gin.Context) ([][]byte, bool) {
	form, err := c.MultipartForm()
	if err != nil {
		writeFail(c, apicode.NoPages, "")
		return nil, false
	}
	files := form.File["pages"]
	uploads := make([][]byte, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			writeFail(c, apicode.InvalidImage, "")
			return nil, false
		}
		data, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			writeFail(c, apicode.InvalidImage, "")
			return nil, false
		}
		uploads = append(uploads, data)
	}
	return uploads, true
}

func parseFlag(c *gin.Context, name string) int {
	raw := c.PostForm(name)
	if raw == "" {
		raw = c.Query(name)
	}
	switch raw {
	case "1", "true", "on":
		return 1
	default:
		return 0
	}
}
