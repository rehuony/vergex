package api

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers WebP with image.Decode
)

const (
	// maxAvatarUpload caps the uploaded file; the web app sends a 256px PNG.
	maxAvatarUpload = 2 << 20
	// maxAvatarSide is checked from the header before decoding, since a tiny
	// file can declare a canvas large enough to exhaust memory.
	maxAvatarSide = 4096
	// avatarSide is the edge of the square the avatar is stored at.
	avatarSide = 256
)

var errAvatarFormat = errors.New("avatar must be a PNG, JPEG or WebP image")

// normalizeAvatar decodes an uploaded image and redraws it as a 256px square,
// returned as a data URL. Only the decoded pixels reach the output, so data
// appended to the file, metadata and anything else riding along is dropped;
// an upload that is not a well-formed PNG, JPEG or WebP is rejected.
func normalizeAvatar(raw []byte) (string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	// The content sniffer must agree with the decoder, so a file that merely
	// starts like an image cannot pass as one.
	if err != nil || http.DetectContentType(raw) != "image/"+format {
		return "", errAvatarFormat
	}
	if cfg.Width > maxAvatarSide || cfg.Height > maxAvatarSide {
		return "", fmt.Errorf("avatar must be at most %d × %d pixels", maxAvatarSide, maxAvatarSide)
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", errAvatarFormat
	}

	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	square := image.Rect(0, 0, side, side).Add(b.Min).Add(image.Pt((b.Dx()-side)/2, (b.Dy()-side)/2))
	dst := image.NewRGBA(image.Rect(0, 0, avatarSide, avatarSide))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, square, draw.Src, nil)

	// JPEG keeps photos small; PNG keeps transparency.
	var out bytes.Buffer
	mime := "image/png"
	if dst.Opaque() {
		mime = "image/jpeg"
		err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(&out, dst)
	}
	if err != nil {
		return "", err
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(out.Bytes()), nil
}
