package chat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"

	"github.com/Scrin/siikabot/db"
	_ "golang.org/x/image/webp"
)

// An image is shown to the model as it is prepared here, and the turn keeps it that way so that it
// is shown the same way for as long as the turn is in the context window. Every image goes through
// the same steps, small ones included: it is turned upright, scaled down if it is large, and encoded
// anew, which leaves its metadata, such as where a photo was taken, behind. What comes out is a JPEG,
// or a PNG if the image has any transparency, which every provider takes.

// maxImageSide is the longest either side of an image may be when the model is shown it. Text in a
// screenshot stays legible at this size, providers scale images down to about this anyway, and a
// phone photo comes out at a few hundred kilobytes rather than several megabytes.
const maxImageSide = 1536

// maxDecodedImageBytes bounds the memory decoding an image may take. A small file can declare
// enormous dimensions, and decoding it would take the bot's memory with it, so the size is read from
// the header first. This lets through photos of over 30 megapixels.
const maxDecodedImageBytes = 128 << 20

// imageJPEGQuality is the quality an image without transparency is encoded at
const imageJPEGQuality = 90

// prepareImage turns an image as it was downloaded into the image the model is shown. JPEG, PNG,
// GIF and WebP images are read; anything else, or anything too large to decode, is an error. The
// returned image has no EventID: whose image it is is for the caller to say.
func prepareImage(data []byte) (db.ChatImage, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return db.ChatImage{}, fmt.Errorf("failed to read image header: %w", err)
	}
	if size := decodedSize(config); config.Width <= 0 || config.Height <= 0 || size > maxDecodedImageBytes {
		return db.ChatImage{}, fmt.Errorf("%s image of %dx%d pixels would take %d bytes to decode, over the limit of %d",
			format, config.Width, config.Height, size, maxDecodedImageBytes)
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return db.ChatImage{}, fmt.Errorf("failed to decode %s image: %w", format, err)
	}

	// Phone cameras store a photo the way the sensor was held, with a tag saying which way is up.
	// The tag doesn't survive encoding the image anew, so the turn is made here.
	orientation := 1
	if format == "jpeg" {
		orientation = jpegOrientation(data)
	}

	img := src
	bounds := src.Bounds()
	if width, height := fitWithin(bounds.Dx(), bounds.Dy(), maxImageSide); width != bounds.Dx() || height != bounds.Dy() {
		img = orient(shrink(src, width, height), orientation)
	} else if orientation != 1 {
		rgba := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.Draw(rgba, rgba.Bounds(), src, bounds.Min, draw.Src)
		img = orient(rgba, orientation)
	}

	var encoded bytes.Buffer
	mimeType := "image/jpeg"
	if isOpaque(src) {
		err = jpeg.Encode(&encoded, img, &jpeg.Options{Quality: imageJPEGQuality})
	} else {
		mimeType = "image/png"
		err = png.Encode(&encoded, img)
	}
	if err != nil {
		return db.ChatImage{}, fmt.Errorf("failed to encode image as %s: %w", mimeType, err)
	}

	return db.ChatImage{
		MIMEType: mimeType,
		Width:    img.Bounds().Dx(),
		Height:   img.Bounds().Dy(),
		Data:     encoded.Bytes(),
	}, nil
}

// decodedSize is about as much memory as decoding an image takes at most: four bytes a pixel, or
// eight for 16 bits a channel
func decodedSize(config image.Config) int64 {
	bytesPerPixel := int64(4)
	switch config.ColorModel {
	case color.RGBA64Model, color.NRGBA64Model, color.Gray16Model:
		bytesPerPixel = 8
	}
	return int64(config.Width) * int64(config.Height) * bytesPerPixel
}

// fitWithin returns the size an image of width×height is scaled to so that neither side is longer
// than limit, keeping its aspect ratio, or its own size if it fits already
func fitWithin(width, height, limit int) (int, int) {
	if width <= limit && height <= limit {
		return width, height
	}
	if width >= height {
		return limit, max(1, int(math.Round(float64(height)*float64(limit)/float64(width))))
	}
	return max(1, int(math.Round(float64(width)*float64(limit)/float64(height)))), limit
}

// isOpaque reports whether an image has no transparency at all. Every image type the decoders
// produce can tell; any other is taken to have some, which keeps it lossless as a PNG.
func isOpaque(img image.Image) bool {
	opaque, ok := img.(interface{ Opaque() bool })
	return ok && opaque.Opaque()
}

// shrink scales an image down to width×height, each new pixel the average of the part of the image
// it covers. It goes a row at a time: a kernel scaler such as x/image/draw's keeps an intermediate
// image as tall as the original, which for a large photo takes a few hundred megabytes.
func shrink(src image.Image, width, height int) *image.RGBA {
	bounds := src.Bounds()
	columns := coverage(bounds.Dx(), width)
	rows := coverage(bounds.Dy(), height)

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	// A row of the original, as RGBA whatever the image's own type
	row := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), 1))
	sum := make([]float64, width*4)
	for y, sourceRows := range rows {
		clear(sum)
		for _, sourceRow := range sourceRows {
			draw.Draw(row, row.Bounds(), src, image.Pt(bounds.Min.X, bounds.Min.Y+sourceRow.index), draw.Src)
			for x, sourceColumns := range columns {
				for _, sourceColumn := range sourceColumns {
					weight := sourceRow.weight * sourceColumn.weight
					pixel := row.Pix[sourceColumn.index*4 : sourceColumn.index*4+4]
					for c, value := range pixel {
						sum[x*4+c] += weight * float64(value)
					}
				}
			}
		}

		out := dst.Pix[y*dst.Stride : y*dst.Stride+width*4]
		for i, value := range sum {
			out[i] = uint8(min(255, math.Round(value)))
		}
	}
	return dst
}

// share is how much of a pixel of a shrunk image a pixel of the original makes up
type share struct {
	index  int
	weight float64
}

// coverage splits a line of length pixels into count pixels, returning for each of them the
// original pixels it covers and their shares of it, which add up to one
func coverage(length, count int) [][]share {
	scale := float64(length) / float64(count)
	shares := make([][]share, count)
	for i := range shares {
		start, end := float64(i)*scale, float64(i+1)*scale
		for j := int(start); j < length && float64(j) < end; j++ {
			if overlap := min(end, float64(j+1)) - max(start, float64(j)); overlap > 0 {
				shares[i] = append(shares[i], share{index: j, weight: overlap / scale})
			}
		}
	}
	return shares
}

// orient turns an image upright from its EXIF orientation (see jpegOrientation). Mirroring and
// turning by a multiple of 90 degrees only move pixels around, so nothing of the image is lost.
func orient(src *image.RGBA, orientation int) *image.RGBA {
	if orientation < 2 || orientation > 8 {
		return src
	}

	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dstWidth, dstHeight := width, height
	if orientation >= 5 {
		// Turned a quarter, so its sides swap
		dstWidth, dstHeight = height, width
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	for y := 0; y < dstHeight; y++ {
		for x := 0; x < dstWidth; x++ {
			srcX, srcY := orientedSource(orientation, width, height, x, y)
			from := src.PixOffset(bounds.Min.X+srcX, bounds.Min.Y+srcY)
			to := dst.PixOffset(x, y)
			copy(dst.Pix[to:to+4], src.Pix[from:from+4])
		}
	}
	return dst
}

// orientedSource returns the pixel of a stored image of width×height that ends up at x, y once the
// image is turned upright from its EXIF orientation
func orientedSource(orientation, width, height, x, y int) (int, int) {
	switch orientation {
	case 2: // mirrored left to right
		return width - 1 - x, y
	case 3: // upside down
		return width - 1 - x, height - 1 - y
	case 4: // mirrored top to bottom
		return x, height - 1 - y
	case 5: // mirrored along the diagonal from the top left
		return y, x
	case 6: // turned a quarter counterclockwise, so it is shown turned a quarter clockwise
		return y, height - 1 - x
	case 7: // mirrored along the diagonal from the top right
		return width - 1 - y, height - 1 - x
	case 8: // turned a quarter clockwise, so it is shown turned a quarter counterclockwise
		return width - 1 - y, x
	}
	return x, y
}

// EXIF data is a TIFF structure in a JPEG's APP1 segment, after this identifier
var exifHeader = []byte("Exif\x00\x00")

// exifOrientationTag is the EXIF tag that says which way is up
const exifOrientationTag = 0x0112

// jpegOrientation reads the EXIF orientation of a JPEG: which of the eight ways it was stored in,
// from 1, upright, to 8. Anything that can't be read, including a JPEG without EXIF, leaves it at 1,
// and the image as it was stored.
//
// It reads only as far as the orientation, so a small reader does where an EXIF library would bring
// a dependency for the rest of the format.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}

	// The segments before the image data, each a marker and a length that counts itself
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		switch marker {
		case 0xFF:
			// Fill before a marker
			i++
			continue
		case 0xDA, 0xD9:
			// The image data or the end, without EXIF before it
			return 1
		}

		length := int(binary.BigEndian.Uint16(data[i+2:]))
		if length < 2 || i+2+length > len(data) {
			return 1
		}
		segment := data[i+4 : i+2+length]
		if marker == 0xE1 && bytes.HasPrefix(segment, exifHeader) {
			return exifOrientation(segment[len(exifHeader):])
		}
		i += 2 + length
	}
	return 1
}

// exifOrientation reads the orientation from EXIF data: a TIFF header giving the byte order and
// where the first directory is, and the directory's entries of 12 bytes each, one of which may be
// the orientation, a 16-bit value
func exifOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(tiff[2:]) != 42 {
		return 1
	}

	directory := int64(order.Uint32(tiff[4:]))
	if directory+2 > int64(len(tiff)) {
		return 1
	}
	start := int(directory)
	entries := int(order.Uint16(tiff[start:]))
	for i := 0; i < entries; i++ {
		entry := start + 2 + i*12
		if entry+12 > len(tiff) {
			return 1
		}
		if order.Uint16(tiff[entry:]) != exifOrientationTag {
			continue
		}
		// A SHORT, type 3, held in the first two bytes of the entry's value field
		if order.Uint16(tiff[entry+2:]) != 3 {
			return 1
		}
		orientation := int(order.Uint16(tiff[entry+8:]))
		if orientation < 1 || orientation > 8 {
			return 1
		}
		return orientation
	}
	return 1
}
