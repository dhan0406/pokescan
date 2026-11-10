// Package imageprep turns an uploaded card photo into the small grayscale
// image Tesseract reads: a band across the top of the card, where the name is.
package imageprep

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // register JPEG decoding
	"image/png"
	"io"
	"math"
)

const (
	// maxPixels rejects images whose header declares more pixels than the
	// app sends (about 1.5 MP), before they are decoded into memory. At 12 MP a
	// 16-bit PNG still decodes to at most 96 MB.
	maxPixels = 12_000_000
	// minSide rejects images too small to hold readable text.
	minSide = 100
)

// Bands is the vertical position of the name band, as a fraction
// of the image height (0 = top, 1 = bottom). Tune these with real photos.
type Bands struct {
	NameTop, NameBottom float64
}

// DefaultBands assumes the photo is cropped to the card's frame.
var DefaultBands = Bands{NameTop: 0.05, NameBottom: 0.135}

// Regions holds the two bands as PNG-encoded grayscale images.
type Regions struct {
	Name []byte
	// Image is the decoded photo, kept so a fallback can look at all of it.
	Image image.Image
}

// ErrBadImage means the upload is not a usable JPEG or PNG.
var ErrBadImage = errors.New("bad image")

// Prepare decodes a JPEG or PNG and returns its name band as
// grayscale PNGs. The format is detected from the content, not a file name.
// Anything else, or an image that is too large or too small, is ErrBadImage.
func Prepare(r io.Reader, b Bands) (Regions, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Regions{}, fmt.Errorf("%w: read: %v", ErrBadImage, err)
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Regions{}, fmt.Errorf("%w: %v", ErrBadImage, err)
	}
	if format != "jpeg" && format != "png" {
		return Regions{}, fmt.Errorf("%w: unsupported format %q", ErrBadImage, format)
	}
	if cfg.Width*cfg.Height > maxPixels {
		return Regions{}, fmt.Errorf("%w: %dx%d is too large", ErrBadImage, cfg.Width, cfg.Height)
	}
	if cfg.Width < minSide || cfg.Height < minSide {
		return Regions{}, fmt.Errorf("%w: %dx%d is too small", ErrBadImage, cfg.Width, cfg.Height)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Regions{}, fmt.Errorf("%w: %v", ErrBadImage, err)
	}

	name, err := band(img, b.NameTop, b.NameBottom)
	if err != nil {
		return Regions{}, err
	}
	return Regions{Name: name, Image: img}, nil
}

// GrayPNG encodes the whole image as a grayscale PNG.
func GrayPNG(img image.Image) ([]byte, error) {
	return cropPNG(img, img.Bounds())
}

// CropPNG encodes the part of img inside r (clamped to the image) as a
// grayscale PNG. It is ErrBadImage if nothing is left.
func CropPNG(img image.Image, r image.Rectangle) ([]byte, error) {
	r = r.Intersect(img.Bounds())
	if r.Dx() < 1 || r.Dy() < 1 {
		return nil, fmt.Errorf("%w: empty crop", ErrBadImage)
	}
	return cropPNG(img, r)
}

func cropPNG(img image.Image, r image.Rectangle) ([]byte, error) {
	gray := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(gray, gray.Bounds(), img, r.Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, gray); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return buf.Bytes(), nil
}

// band crops the full-width rows between the top and bottom fractions of the
// image height, converts them to grayscale and encodes them as PNG.
func band(img image.Image, top, bottom float64) ([]byte, error) {
	bounds := img.Bounds()
	h := float64(bounds.Dy())
	rect := image.Rect(
		bounds.Min.X, bounds.Min.Y+int(math.Round(top*h)),
		bounds.Max.X, bounds.Min.Y+int(math.Round(bottom*h)),
	).Intersect(bounds)
	if rect.Dy() < 1 {
		return nil, fmt.Errorf("%w: empty band", ErrBadImage)
	}

	gray := image.NewGray(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(gray, gray.Bounds(), img, rect.Min, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, gray); err != nil {
		return nil, fmt.Errorf("encode band: %w", err)
	}
	return buf.Bytes(), nil
}
