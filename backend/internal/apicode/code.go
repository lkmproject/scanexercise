package apicode

import "net/http"

// Code is the business status of every JSON API response.
// 200 means success; any other value means failure.
type Code int32

const (
	OK Code = 200

	// 10xx auth
	AuthInvalid     Code = 1001
	AuthUnavailable Code = 1002
	AuthFailed      Code = 1003
	Unauthorized    Code = 1004

	// 11xx request / upload
	InvalidRequest Code = 1101
	NoPages        Code = 1102
	InvalidImage   Code = 1103
	TooManyPages   Code = 1104
	ImageTooLarge  Code = 1105

	// 12xx jobs
	JobNotFound  Code = 1201
	JobNotReady  Code = 1202
	JobFailed    Code = 1203
	PageNotFound Code = 1204

	// 13xx files
	PDFFailed Code = 1301

	// 14xx credits
	InsufficientCredits Code = 1401

	// 19xx server
	Internal Code = 1901
)

func (c Code) HTTPStatus() int {
	return http.StatusOK
}

func (c Code) Reason() string {
	switch c {
	case OK:
		return "success"
	case AuthInvalid:
		return "invalid google sign-in"
	case AuthUnavailable:
		return "auth service unavailable"
	case AuthFailed:
		return "sign-in failed"
	case Unauthorized:
		return "unauthorized"
	case InvalidRequest:
		return "invalid request"
	case NoPages:
		return "no pages uploaded"
	case InvalidImage:
		return "cannot read image"
	case TooManyPages:
		return "too many pages"
	case ImageTooLarge:
		return "image too large"
	case JobNotFound:
		return "job not found or expired"
	case JobNotReady:
		return "result not ready"
	case JobFailed:
		return "model call failed"
	case PageNotFound:
		return "page not found"
	case PDFFailed:
		return "cannot create pdf"
	case InsufficientCredits:
		return "not enough erases"
	default:
		return "internal error"
	}
}
