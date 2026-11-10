package imageprep

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"
)

// rowShaded returns a w×h image whose row y is gray level y/3.
func rowShaded(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		v := uint8(y / 3)
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeRegion(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("region is not a PNG: %v", err)
	}
	return img
}

func TestPrepare_cropsBands(t *testing.T) {
	r, err := Prepare(bytes.NewReader(encodePNG(t, rowShaded(500, 700))), DefaultBands)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	tests := []struct {
		name        string
		png         []byte
		top, bottom int // source rows
	}{
		{"name", r.Name, 28, 84},
		{"number", r.Number, 644, 686},
	}
	for _, tt := range tests {
		img := decodeRegion(t, tt.png)
		b := img.Bounds()
		if b.Dx() != 500 || b.Dy() != tt.bottom-tt.top {
			t.Errorf("%s band is %dx%d, want 500x%d", tt.name, b.Dx(), b.Dy(), tt.bottom-tt.top)
			continue
		}
		for _, dy := range []int{0, b.Dy() - 1} {
			got := color.GrayModel.Convert(img.At(b.Min.X+250, b.Min.Y+dy)).(color.Gray).Y
			want := uint8((tt.top + dy) / 3)
			if got != want {
				t.Errorf("%s band row %d = gray %d, want %d (source row %d)", tt.name, dy, got, want, tt.top+dy)
			}
		}
	}
}

func TestPrepare_acceptsJPEGAndPNG(t *testing.T) {
	var jp bytes.Buffer
	if err := jpeg.Encode(&jp, rowShaded(500, 700), nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"png": encodePNG(t, rowShaded(500, 700)), "jpeg": jp.Bytes()} {
		r, err := Prepare(bytes.NewReader(data), DefaultBands)
		if err != nil {
			t.Errorf("%s: Prepare: %v", name, err)
			continue
		}
		if decodeRegion(t, r.Name).Bounds().Dy() != 56 {
			t.Errorf("%s: name band height wrong", name)
		}
	}
}

func TestPrepare_outputIsGrayPNG(t *testing.T) {
	r, err := Prepare(bytes.NewReader(encodePNG(t, rowShaded(500, 700))), DefaultBands)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	for _, b := range [][]byte{r.Name, r.Number} {
		if _, ok := decodeRegion(t, b).(*image.Gray); !ok {
			t.Errorf("region decodes as %T, want *image.Gray", decodeRegion(t, b))
		}
	}
}

func TestPrepare_rejectsNonImage(t *testing.T) {
	var g bytes.Buffer
	if err := gif.Encode(&g, rowShaded(500, 700), nil); err != nil {
		t.Fatal(err)
	}
	full := encodePNG(t, rowShaded(500, 700))
	inputs := map[string][]byte{
		"text":          []byte("hello"),
		"empty":         nil,
		"gif":           g.Bytes(),
		"truncated png": full[:100],
	}
	for name, data := range inputs {
		if _, err := Prepare(bytes.NewReader(data), DefaultBands); !errors.Is(err, ErrBadImage) {
			t.Errorf("%s: err = %v, want ErrBadImage", name, err)
		}
	}
}

// hugePNG builds a valid PNG signature and IHDR declaring w×h pixels, followed
// by no image data. Decoding the header is cheap; decoding the image would not be.
func hugePNG(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 0 // 8-bit grayscale
	binary.Write(&buf, binary.BigEndian, uint32(len(ihdr)))
	chunk := append([]byte("IHDR"), ihdr...)
	buf.Write(chunk)
	binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return buf.Bytes()
}

func TestPrepare_rejectsHugeDimensions(t *testing.T) {
	start := time.Now()
	_, err := Prepare(bytes.NewReader(hugePNG(50000, 50000)), DefaultBands)
	if !errors.Is(err, ErrBadImage) {
		t.Errorf("err = %v, want ErrBadImage", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v, want under 1s (should reject from the header)", d)
	}
}

func TestPrepare_rejectsTinyImage(t *testing.T) {
	_, err := Prepare(bytes.NewReader(encodePNG(t, rowShaded(10, 10))), DefaultBands)
	if !errors.Is(err, ErrBadImage) {
		t.Errorf("err = %v, want ErrBadImage", err)
	}
}

func TestPrepare_rejectsOverTwelveMegapixels(t *testing.T) {
	// 4000x4000 is 16 MP: far beyond the ~1.5 MP the app sends, and as a
	// 16-bit PNG it would decode to 128 MB.
	if _, err := Prepare(bytes.NewReader(hugePNG(4000, 4000)), DefaultBands); !errors.Is(err, ErrBadImage) || !strings.Contains(err.Error(), "too large") {
		t.Errorf("err = %v, want ErrBadImage for being too large", err)
	}
}
