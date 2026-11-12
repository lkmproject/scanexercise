package preprocess

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func jpegColor(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 180, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 99}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestToJPEGKeepsSmallJPEG(t *testing.T) {
	src := jpegColor(400, 300)
	out, err := ToJPEG(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, out) {
		t.Fatal("textin jpeg under 1920 should not be re-encoded")
	}
}

func TestToJPEGLimitsLongEdge(t *testing.T) {
	out, err := ToJPEG(jpegColor(3000, 2000))
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != textInMaxLongEdge {
		t.Fatalf("width %d", img.Bounds().Dx())
	}
}

func TestForTextInLimitsLongEdge(t *testing.T) {
	out, err := ForTextIn(jpegColor(3000, 2000))
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	long := b.Dx()
	if b.Dy() > long {
		long = b.Dy()
	}
	if long > textInMaxLongEdge {
		t.Fatalf("long edge %d", long)
	}
}

func TestForTextInSkipsAlreadySmallJPEG(t *testing.T) {
	src := jpegColor(400, 300)
	out, err := ForTextIn(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, out) {
		t.Fatal("already small jpeg should not be re-encoded")
	}
}

func TestPreparePageKeepsHigherCap(t *testing.T) {
	out, err := PreparePage(jpegColor(3000, 2000), 2560)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 2560 {
		t.Fatalf("width %d", img.Bounds().Dx())
	}
}
