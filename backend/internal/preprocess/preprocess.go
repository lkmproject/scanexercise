package preprocess

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"

	"github.com/disintegration/imaging"
)

func PreparePage(data []byte, maxLongEdge int) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("無法讀取圖片")
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, errors.New("無法讀取圖片")
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	long := width
	if height > long {
		long = height
	}
	if maxLongEdge > 0 && long > maxLongEdge {
		nextWidth := max(int(float64(width)*float64(maxLongEdge)/float64(long)), 1)
		nextHeight := max(int(float64(height)*float64(maxLongEdge)/float64(long)), 1)
		img = imaging.Resize(img, nextWidth, nextHeight, imaging.Lanczos)
	}
	return encodeJPEG(img)
}

func ToJPEG(data []byte) ([]byte, error) {
	img, err := imaging.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return encodeJPEGAt(img, 90)
}

func encodeJPEG(img image.Image) ([]byte, error) {
	quality := 90
	encoded, err := encodeJPEGAt(img, quality)
	if err != nil {
		return nil, err
	}
	for len(encoded) > 4_000_000 && quality > 50 {
		quality -= 10
		encoded, err = encodeJPEGAt(img, quality)
		if err != nil {
			return nil, err
		}
	}
	return encoded, nil
}

func encodeJPEGAt(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
