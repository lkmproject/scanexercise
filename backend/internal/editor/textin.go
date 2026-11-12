package editor

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"scanexercise/internal/preprocess"
)

const editURL = "https://api.textin.com/ai/service/v1/handwritten_erase"

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type TextIn struct {
	AppID        string
	SecretCode   string
	Timeout      time.Duration
	Crop         int
	Dewarp       int
	Binarization int
	ImageType    int
	DocDirection int
	Client       HTTPClient
}

func NewTextIn(appID, secretCode string, timeoutSeconds int) *TextIn {
	return &TextIn{
		AppID:        appID,
		SecretCode:   secretCode,
		Timeout:      time.Duration(timeoutSeconds) * time.Second,
		Crop:         0,
		Dewarp:       0,
		Binarization: 0,
		ImageType:    1,
		DocDirection: 0,
	}
}

func (t *TextIn) Edit(jpeg []byte, opts Options) ([]byte, error) {
	if t.AppID == "" || t.SecretCode == "" {
		return nil, Fail("TEXTIN_APP_ID or TEXTIN_SECRET_CODE is not set")
	}
	crop := 0
	if opts.Crop == 1 {
		crop = 1
	}
	binarization := 0
	if opts.Binarization == 1 {
		binarization = 1
	}
	query := url.Values{}
	query.Set("crop", strconv.Itoa(crop))                    // 0 skip crop, 1 crop
	query.Set("dewarp", strconv.Itoa(t.Dewarp))              // 0 skip dewarp
	query.Set("binarization", strconv.Itoa(binarization))    // 0 skip, 1 sharpen
	query.Set("image_type", strconv.Itoa(t.ImageType))       // 1 return color image
	query.Set("doc_direction", strconv.Itoa(t.DocDirection)) // 0 skip orientation
	requestImage, err := preprocess.ForTextIn(jpeg)
	if err != nil {
		return nil, Fail("cannot read image")
	}
	req, err := http.NewRequest(http.MethodPost, editURL+"?"+query.Encode(), bytes.NewReader(requestImage))
	if err != nil {
		return nil, Fail("model call failed")
	}
	req.Header.Set("x-ti-app-id", t.AppID)
	req.Header.Set("x-ti-secret-code", t.SecretCode)
	req.Header.Set("Content-Type", "application/octet-stream")

	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: t.Timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		if isTimeout(err) {
			return nil, Fail("model timed out")
		}
		return nil, Fail("model call failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, Fail("model call failed")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Fail("model call failed")
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, Fail("model call failed")
	}
	raw, err := processedImage(payload)
	if err != nil {
		return nil, err
	}
	encoded, err := preprocess.ToJPEG(raw)
	if err != nil {
		return nil, Fail("model returned no image")
	}
	return encoded, nil
}

func processedImage(payload any) ([]byte, error) {
	root, ok := payload.(map[string]any)
	if !ok {
		return nil, Fail("model returned no image")
	}
	if code := root["code"]; code != nil && !isOKCode(code) {
		return nil, Fail("model call failed")
	}
	result, ok := root["result"].(map[string]any)
	if !ok {
		return nil, Fail("model returned no image")
	}
	encoded, ok := result["image"].(string)
	if !ok || encoded == "" {
		return nil, Fail("model returned no image")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, Fail("model returned no image")
	}
	return raw, nil
}

func isOKCode(code any) bool {
	switch value := code.(type) {
	case float64:
		return value == 200
	case string:
		return value == "200"
	default:
		return false
	}
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, http.ErrHandlerTimeout)
}
