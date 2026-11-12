package editor

type Options struct {
	Crop         int
	Binarization int
}

type Editor interface {
	Edit(jpeg []byte, opts Options) ([]byte, error)
}

type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func Fail(message string) error {
	return &Error{Message: message}
}
