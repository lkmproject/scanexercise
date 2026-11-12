package preprocess

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"

	xdraw "golang.org/x/image/draw"
)

const (
	jpegQuality       = 99
	textInMaxLongEdge = 1920
	textInQuality     = 95
	textInMaxBytes    = 3_000_000
	textInMinQuality  = 70
)

func PreparePage(data []byte, maxLongEdge int) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("cannot read image")
	}
	img, err := decodeImage(data, true)
	if err != nil {
		return nil, errors.New("cannot read image")
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
		img = resize(img, nextWidth, nextHeight)
	}
	return encodeJPEG(img)
}

func ToJPEG(data []byte) ([]byte, error) {
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err == nil {
			long := cfg.Width
			if cfg.Height > long {
				long = cfg.Height
			}
			if long > 0 && long <= textInMaxLongEdge {
				return data, nil
			}
		}
	}
	img, err := decodeImage(data, false)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	long := width
	if height > long {
		long = height
	}
	if long > textInMaxLongEdge {
		nextWidth := max(int(float64(width)*float64(textInMaxLongEdge)/float64(long)), 1)
		nextHeight := max(int(float64(height)*float64(textInMaxLongEdge)/float64(long)), 1)
		img = resize(img, nextWidth, nextHeight)
	}
	return encodeJPEGAt(img, jpegQuality)
}

func ForTextIn(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("cannot read image")
	}
	if len(data) <= textInMaxBytes {
		orient := jpegOrientation(data)
		if orient <= 1 {
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err == nil {
				long := cfg.Width
				if cfg.Height > long {
					long = cfg.Height
				}
				if long > 0 && long <= textInMaxLongEdge {
					return data, nil
				}
			}
		}
	}
	img, err := decodeImage(data, true)
	if err != nil {
		return nil, errors.New("cannot read image")
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	long := width
	if height > long {
		long = height
	}
	if long > textInMaxLongEdge {
		nextWidth := max(int(float64(width)*float64(textInMaxLongEdge)/float64(long)), 1)
		nextHeight := max(int(float64(height)*float64(textInMaxLongEdge)/float64(long)), 1)
		img = resize(img, nextWidth, nextHeight)
	}
	quality := textInQuality
	encoded, err := encodeJPEGAt(img, quality)
	if err != nil {
		return nil, err
	}
	for len(encoded) > textInMaxBytes && quality > textInMinQuality {
		quality -= 5
		encoded, err = encodeJPEGAt(img, quality)
		if err != nil {
			return nil, err
		}
	}
	return encoded, nil
}

func decodeImage(data []byte, autoOrient bool) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if autoOrient {
		img = applyOrientation(img, jpegOrientation(data))
	}
	return img, nil
}

func resize(src image.Image, width, height int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

func encodeJPEG(img image.Image) ([]byte, error) {
	return encodeJPEGAt(img, jpegQuality)
}

func encodeJPEGAt(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func jpegOrientation(data []byte) int {
	r := bytes.NewReader(data)
	var soi uint16
	if err := binary.Read(r, binary.BigEndian, &soi); err != nil || soi != 0xffd8 {
		return 0
	}
	for {
		var marker, size uint16
		if err := binary.Read(r, binary.BigEndian, &marker); err != nil {
			return 0
		}
		if err := binary.Read(r, binary.BigEndian, &size); err != nil {
			return 0
		}
		if marker>>8 != 0xff || size < 2 {
			return 0
		}
		if marker != 0xffe1 {
			if _, err := io.CopyN(io.Discard, r, int64(size-2)); err != nil {
				return 0
			}
			continue
		}
		var header uint32
		if err := binary.Read(r, binary.BigEndian, &header); err != nil || header != 0x45786966 {
			return 0
		}
		if _, err := io.CopyN(io.Discard, r, 2); err != nil {
			return 0
		}
		var byteOrderTag uint16
		if err := binary.Read(r, binary.BigEndian, &byteOrderTag); err != nil {
			return 0
		}
		var order binary.ByteOrder
		switch byteOrderTag {
		case 0x4d4d:
			order = binary.BigEndian
		case 0x4949:
			order = binary.LittleEndian
		default:
			return 0
		}
		if _, err := io.CopyN(io.Discard, r, 2); err != nil {
			return 0
		}
		var offset uint32
		if err := binary.Read(r, order, &offset); err != nil || offset < 8 {
			return 0
		}
		if _, err := io.CopyN(io.Discard, r, int64(offset-8)); err != nil {
			return 0
		}
		var numTags uint16
		if err := binary.Read(r, order, &numTags); err != nil {
			return 0
		}
		for i := 0; i < int(numTags); i++ {
			var tag uint16
			if err := binary.Read(r, order, &tag); err != nil {
				return 0
			}
			if tag != 0x0112 {
				if _, err := io.CopyN(io.Discard, r, 10); err != nil {
					return 0
				}
				continue
			}
			if _, err := io.CopyN(io.Discard, r, 6); err != nil {
				return 0
			}
			var val uint16
			if err := binary.Read(r, order, &val); err != nil || val < 1 || val > 8 {
				return 0
			}
			return int(val)
		}
		return 0
	}
}

func applyOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return flipH(img)
	case 3:
		return rotate180(img)
	case 4:
		return flipV(img)
	case 5:
		return transpose(img)
	case 6:
		return rotate270(img)
	case 7:
		return transverse(img)
	case 8:
		return rotate90(img)
	default:
		return img
	}
}

func toNRGBA(src image.Image) *image.NRGBA {
	if nrgba, ok := src.(*image.NRGBA); ok && nrgba.Rect.Min.Eq(image.Point{}) {
		return nrgba
	}
	bounds := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	xdraw.Draw(dst, dst.Bounds(), src, bounds.Min, xdraw.Src)
	return dst
}

func flipH(src image.Image) image.Image {
	in := toNRGBA(src)
	w, h := in.Bounds().Dx(), in.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(w-1-x, y, in.NRGBAAt(x, y))
		}
	}
	return out
}

func flipV(src image.Image) image.Image {
	in := toNRGBA(src)
	w, h := in.Bounds().Dx(), in.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(x, h-1-y, in.NRGBAAt(x, y))
		}
	}
	return out
}

func rotate180(src image.Image) image.Image {
	return flipH(flipV(src))
}

func rotate90(src image.Image) image.Image {
	in := toNRGBA(src)
	w, h := in.Bounds().Dx(), in.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(y, w-1-x, in.NRGBAAt(x, y))
		}
	}
	return out
}

func rotate270(src image.Image) image.Image {
	in := toNRGBA(src)
	w, h := in.Bounds().Dx(), in.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetNRGBA(h-1-y, x, in.NRGBAAt(x, y))
		}
	}
	return out
}

func transpose(src image.Image) image.Image {
	return rotate90(flipH(src))
}

func transverse(src image.Image) image.Image {
	return rotate90(flipV(src))
}
