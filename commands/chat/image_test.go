package chat

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// solidImage is an opaque image of one colour
func solidImage(width, height int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// decoded decodes a prepared image, checking that its type says what it is
func decoded(t *testing.T, data []byte, mimeType string) image.Image {
	t.Helper()
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("prepared image doesn't decode: %v", err)
	}
	if "image/"+format != mimeType {
		t.Errorf("prepared image is %s, but its type says %s", format, mimeType)
	}
	return img
}

// exifSegment builds a JPEG APP1 segment holding EXIF data with nothing but an orientation, in the
// given byte order
func exifSegment(order binary.ByteOrder, orientation uint16) []byte {
	tiff := make([]byte, 26)
	if order == binary.LittleEndian {
		copy(tiff, "II")
	} else {
		copy(tiff, "MM")
	}
	order.PutUint16(tiff[2:], 42)
	order.PutUint32(tiff[4:], 8) // the directory right after the header
	order.PutUint16(tiff[8:], 1) // one entry
	order.PutUint16(tiff[10:], exifOrientationTag)
	order.PutUint16(tiff[12:], 3) // a SHORT
	order.PutUint32(tiff[14:], 1) // one of them
	order.PutUint16(tiff[18:], orientation)

	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(payload)+2))
	return append(segment, payload...)
}

// withSegment puts a segment into a JPEG right after its start marker, where a camera writes EXIF
func withSegment(jpegData, segment []byte) []byte {
	out := append([]byte{}, jpegData[:2]...)
	out = append(out, segment...)
	return append(out, jpegData[2:]...)
}

// pngHeader is the start of a PNG declaring an image of the given size, which is all that reading
// its size takes
func pngHeader(width, height uint32, bitDepth, colorType byte) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], width)
	binary.BigEndian.PutUint32(ihdr[4:], height)
	ihdr[8], ihdr[9] = bitDepth, colorType

	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(ihdr)))
	chunk = append(chunk, "IHDR"...)
	chunk = append(chunk, ihdr...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	return append([]byte("\x89PNG\r\n\x1a\n"), chunk...)
}

func TestPrepareImageShrinksALargeImage(t *testing.T) {
	prepared, err := prepareImage(encodePNG(t, solidImage(3072, 1536, color.RGBA{200, 30, 30, 255})))
	if err != nil {
		t.Fatal(err)
	}

	if prepared.Width != 1536 || prepared.Height != 768 {
		t.Errorf("prepared image is %dx%d, want 1536x768", prepared.Width, prepared.Height)
	}
	img := decoded(t, prepared.Data, prepared.MIMEType)
	if size := img.Bounds().Size(); size.X != prepared.Width || size.Y != prepared.Height {
		t.Errorf("prepared image decodes as %v, but says it is %dx%d", size, prepared.Width, prepared.Height)
	}
	// An opaque image comes out as a JPEG, even from a PNG
	if prepared.MIMEType != "image/jpeg" {
		t.Errorf("an opaque image came out as %s, want image/jpeg", prepared.MIMEType)
	}
}

func TestPrepareImageKeepsTheSizeOfASmallImage(t *testing.T) {
	prepared, err := prepareImage(encodeJPEG(t, solidImage(200, 100, color.RGBA{30, 200, 30, 255})))
	if err != nil {
		t.Fatal(err)
	}

	if prepared.Width != 200 || prepared.Height != 100 || prepared.MIMEType != "image/jpeg" {
		t.Errorf("prepared image is a %dx%d %s, want a 200x100 image/jpeg", prepared.Width, prepared.Height, prepared.MIMEType)
	}
	decoded(t, prepared.Data, prepared.MIMEType)
}

func TestPrepareImageKeepsTransparency(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2000, 1000))
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})

	prepared, err := prepareImage(encodePNG(t, img))
	if err != nil {
		t.Fatal(err)
	}

	if prepared.MIMEType != "image/png" {
		t.Errorf("an image with transparency came out as %s, want image/png", prepared.MIMEType)
	}
	if isOpaque(decoded(t, prepared.Data, prepared.MIMEType)) {
		t.Error("the prepared image lost its transparency")
	}
	if prepared.Width != 1536 || prepared.Height != 768 {
		t.Errorf("prepared image is %dx%d, want 1536x768", prepared.Width, prepared.Height)
	}
}

// A phone photo stored sideways, with a tag saying it is to be shown turned a quarter clockwise,
// comes out upright: its left half, red, ends up on top
func TestPrepareImageTurnsAPhotoUpright(t *testing.T) {
	stored := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			stored.Set(x, y, color.RGBA{255, 0, 0, 255})
			if x >= 20 {
				stored.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}

	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		prepared, err := prepareImage(withSegment(encodeJPEG(t, stored), exifSegment(order, 6)))
		if err != nil {
			t.Fatal(err)
		}

		if prepared.Width != 20 || prepared.Height != 40 {
			t.Fatalf("%v: prepared image is %dx%d, want 20x40", order, prepared.Width, prepared.Height)
		}
		img := decoded(t, prepared.Data, prepared.MIMEType)
		top, bottom := img.At(10, 5), img.At(10, 35)
		if r, _, b, _ := top.RGBA(); r < b {
			t.Errorf("%v: the top of the upright image is %v, want red", order, top)
		}
		if r, _, b, _ := bottom.RGBA(); b < r {
			t.Errorf("%v: the bottom of the upright image is %v, want blue", order, bottom)
		}
	}
}

// Nothing of the original's metadata is kept, such as where a photo was taken
func TestPrepareImageLeavesTheMetadataBehind(t *testing.T) {
	original := withSegment(encodeJPEG(t, solidImage(64, 64, color.RGBA{30, 30, 200, 255})), exifSegment(binary.BigEndian, 1))
	if !bytes.Contains(original, []byte("Exif")) {
		t.Fatal("the test image has no EXIF to begin with")
	}

	prepared, err := prepareImage(original)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(prepared.Data, []byte("Exif")) {
		t.Error("the prepared image kept its EXIF")
	}
}

// An image is refused from its header alone when decoding it would take too much memory, before
// anything is decoded: the header is all there is of these
func TestPrepareImageRefusesAnImageTooLargeToDecode(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"enormous", pngHeader(100_000, 100_000, 8, 6)},
		{"just over the limit", pngHeader(6000, 6000, 8, 2)},
		// Twice the memory a pixel, so half the pixels are enough
		{"16 bits a channel", pngHeader(4500, 4500, 16, 6)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := prepareImage(tt.data); err == nil || !strings.Contains(err.Error(), "over the limit") {
				t.Errorf("prepareImage() error = %v, want it refused for its size", err)
			}
		})
	}
}

func TestPrepareImageRefusesWhatIsNoImageItCanRead(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"text", []byte("this is not an image")},
		{"a BMP", append([]byte("BM"), make([]byte, 64)...)},
		{"a truncated PNG", encodePNG(t, solidImage(64, 64, color.Black))[:100]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := prepareImage(tt.data); err == nil {
				t.Error("prepareImage() succeeded, want an error")
			}
		})
	}
}

func TestDecodedSize(t *testing.T) {
	if got := decodedSize(image.Config{ColorModel: color.RGBAModel, Width: 10, Height: 20}); got != 800 {
		t.Errorf("decodedSize() of 8 bits a channel = %d, want 800", got)
	}
	if got := decodedSize(image.Config{ColorModel: color.NRGBA64Model, Width: 10, Height: 20}); got != 1600 {
		t.Errorf("decodedSize() of 16 bits a channel = %d, want 1600", got)
	}
}

func TestFitWithin(t *testing.T) {
	for _, tt := range []struct {
		width, height         int
		wantWidth, wantHeight int
	}{
		{1000, 500, 1000, 500},
		{1536, 1536, 1536, 1536},
		{3072, 1536, 1536, 768},
		{1536, 3072, 768, 1536},
		{4032, 3024, 1536, 1152},
		{1537, 1537, 1536, 1536},
		{10_000, 1, 1536, 1},
	} {
		width, height := fitWithin(tt.width, tt.height, maxImageSide)
		if width != tt.wantWidth || height != tt.wantHeight {
			t.Errorf("fitWithin(%d, %d) = %d, %d, want %d, %d", tt.width, tt.height, width, height, tt.wantWidth, tt.wantHeight)
		}
	}
}

// Each new pixel is the average of the ones it covers, so stripes too fine to keep turn grey rather
// than into a pattern that isn't there
func TestShrinkAveragesThePixelsItCovers(t *testing.T) {
	stripes := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			stripes.Set(x, y, color.Black)
			if x%2 == 1 {
				stripes.Set(x, y, color.White)
			}
		}
	}

	shrunk := shrink(stripes, 2, 1)
	for x := 0; x < 2; x++ {
		if got, want := shrunk.RGBAAt(x, 0), (color.RGBA{128, 128, 128, 255}); got != want {
			t.Errorf("pixel %d = %v, want %v", x, got, want)
		}
	}

	if got, want := shrink(solidImage(9, 7, color.RGBA{10, 20, 30, 255}), 4, 3).RGBAAt(3, 2), (color.RGBA{10, 20, 30, 255}); got != want {
		t.Errorf("a solid colour shrunk to %v, want %v", got, want)
	}
}

func TestCoverageSharesAddUpToOne(t *testing.T) {
	for _, tt := range []struct{ length, count int }{{4, 2}, {3, 2}, {4032, 1536}, {1537, 1536}, {7, 1}} {
		shares := coverage(tt.length, tt.count)
		if len(shares) != tt.count {
			t.Fatalf("coverage(%d, %d) has %d pixels", tt.length, tt.count, len(shares))
		}
		for i, pixel := range shares {
			total := 0.0
			for _, share := range pixel {
				if share.index < 0 || share.index >= tt.length {
					t.Fatalf("coverage(%d, %d) pixel %d covers pixel %d of the original", tt.length, tt.count, i, share.index)
				}
				total += share.weight
			}
			if total < 0.999 || total > 1.001 {
				t.Errorf("coverage(%d, %d) pixel %d shares add up to %f", tt.length, tt.count, i, total)
			}
		}
	}
}

// Each orientation is turned upright as EXIF defines it. The stored image is three pixels wide and
// two tall:
//
//	A B C
//	D E F
func TestOrient(t *testing.T) {
	stored := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i, letter := range "ABCDEF" {
		stored.SetRGBA(i%3, i/3, color.RGBA{uint8(letter), 0, 0, 255})
	}
	read := func(img *image.RGBA) string {
		var rows []string
		for y := 0; y < img.Bounds().Dy(); y++ {
			row := ""
			for x := 0; x < img.Bounds().Dx(); x++ {
				row += string(rune(img.RGBAAt(x, y).R))
			}
			rows = append(rows, row)
		}
		return strings.Join(rows, " ")
	}

	for orientation, want := range map[int]string{
		1: "ABC DEF",
		2: "CBA FED",
		3: "FED CBA",
		4: "DEF ABC",
		5: "AD BE CF",
		6: "DA EB FC",
		7: "FC EB DA",
		8: "CF BE AD",
	} {
		if got := read(orient(stored, orientation)); got != want {
			t.Errorf("orientation %d turned out as %q, want %q", orientation, got, want)
		}
	}
}

func TestJPEGOrientation(t *testing.T) {
	plain := encodeJPEG(t, solidImage(8, 8, color.White))
	// The JFIF segment most JPEGs open with, to read past on the way to the EXIF
	jfif := []byte{0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}

	for _, tt := range []struct {
		name string
		data []byte
		want int
	}{
		{"no EXIF", plain, 1},
		{"little endian", withSegment(plain, exifSegment(binary.LittleEndian, 6)), 6},
		{"big endian", withSegment(plain, exifSegment(binary.BigEndian, 8)), 8},
		{"after a JFIF segment", withSegment(withSegment(plain, exifSegment(binary.BigEndian, 3)), jfif), 3},
		{"an orientation that doesn't exist", withSegment(plain, exifSegment(binary.BigEndian, 9)), 1},
		{"EXIF cut short", withSegment(plain, exifSegment(binary.BigEndian, 6))[:20], 1},
		{"not a JPEG", []byte("GIF89a"), 1},
		{"nothing", nil, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := jpegOrientation(tt.data); got != tt.want {
				t.Errorf("jpegOrientation() = %d, want %d", got, tt.want)
			}
		})
	}
}

// EXIF data whose directory doesn't hold an orientation leaves the image as it is, and so does one
// that points somewhere it can't be
func TestExifOrientationWithoutAnOrientation(t *testing.T) {
	segment := exifSegment(binary.LittleEndian, 6)
	tiff := segment[4+len(exifHeader):]

	other := append([]byte{}, tiff...)
	binary.LittleEndian.PutUint16(other[10:], 0x010F) // the camera's make instead
	if got := exifOrientation(other); got != 1 {
		t.Errorf("exifOrientation() without an orientation = %d, want 1", got)
	}

	elsewhere := append([]byte{}, tiff...)
	binary.LittleEndian.PutUint32(elsewhere[4:], 0xFFFFFFF0)
	if got := exifOrientation(elsewhere); got != 1 {
		t.Errorf("exifOrientation() with its directory out of bounds = %d, want 1", got)
	}
}
