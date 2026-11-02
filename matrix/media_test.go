package matrix

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"testing"
)

// testPNG is a small PNG image, the kind of file that must always come out of decryption readable
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for x := range 4 {
		for y := range 3 {
			img.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 80), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}
	return buf.Bytes()
}

// encryptFile encrypts a file the way a Matrix client does, with the standard library rather than
// the code under test, and returns it with the file object its event would carry, as that object
// reads once parsed from JSON
func encryptFile(t *testing.T, plaintext []byte, key [32]byte, iv [16]byte) ([]byte, map[string]any) {
	t.Helper()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatalf("aes.NewCipher() error = %v", err)
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCTR(block, iv[:]).XORKeyStream(ciphertext, plaintext)
	hash := sha256.Sum256(ciphertext)

	return ciphertext, map[string]any{
		"url": "mxc://example.com/file",
		"key": map[string]any{
			"kty":     "oct",
			"key_ops": []any{"encrypt", "decrypt"},
			"alg":     "A256CTR",
			"k":       base64.RawURLEncoding.EncodeToString(key[:]),
			"ext":     true,
		},
		"iv":     base64.RawStdEncoding.EncodeToString(iv[:]),
		"hashes": map[string]any{"sha256": base64.RawStdEncoding.EncodeToString(hash[:])},
		"v":      "v2",
	}
}

// assertDecryptsToPNG decrypts a file and checks that the PNG it was made from comes back
func assertDecryptsToPNG(t *testing.T, ciphertext []byte, fileInfo map[string]any, want []byte) {
	t.Helper()
	data := bytes.Clone(ciphertext)
	if err := decryptAttachment(data, fileInfo); err != nil {
		t.Fatalf("decryptAttachment() with iv %v error = %v", fileInfo["iv"], err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("decryptAttachment() with iv %v did not give back the original file", fileInfo["iv"])
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatalf("decrypted file with iv %v is not a readable PNG: %v", fileInfo["iv"], err)
	}
}

// A PNG from 2 Nov 2026 came out of decryption as noise. Its IV had a "+" in it, which the bot
// took for a separator and decoded only the part of the IV before it.
func TestDecryptAttachmentWithPlusInIV(t *testing.T) {
	iv := [16]byte{0xce, 0x2d, 0x89, 0x8e, 0x50, 0xe2, 0xf8, 0x7a}
	if encoded := base64.RawStdEncoding.EncodeToString(iv[:]); encoded != "zi2JjlDi+HoAAAAAAAAAAA" {
		t.Fatalf("test IV encodes as %q, not as the IV of the failed image", encoded)
	}
	var key [32]byte
	for i := range key {
		key[i] = byte(i * 7)
	}

	want := testPNG(t)
	ciphertext, fileInfo := encryptFile(t, want, key, iv)
	assertDecryptsToPNG(t, ciphertext, fileInfo, want)
}

// Clients pick the key and the first half of the IV at random, so whatever characters their
// encodings happen to contain, every file has to decrypt. The old decoding got about one in eleven
// wrong, so a few thousand files are enough to catch anything like it.
func TestDecryptAttachmentWithAnyKeyAndIV(t *testing.T) {
	rng := rand.New(rand.NewPCG(2026, 1102))
	want := testPNG(t)
	for range 3000 {
		var key [32]byte
		var iv [16]byte
		for i := range key {
			key[i] = byte(rng.Uint32())
		}
		// Like Element, only the first half of the IV is random, so the counter can't wrap
		for i := range 8 {
			iv[i] = byte(rng.Uint32())
		}
		ciphertext, fileInfo := encryptFile(t, want, key, iv)
		assertDecryptsToPNG(t, ciphertext, fileInfo, want)
	}
}

// The spec leaves the padding off, but a client that adds it means the same values
func TestDecryptAttachmentWithPaddedValues(t *testing.T) {
	want := testPNG(t)
	ciphertext, fileInfo := encryptFile(t, want, [32]byte{1, 2, 3}, [16]byte{0xfb, 0xff, 0x01})
	key := fileInfo["key"].(map[string]any)
	key["k"] = key["k"].(string) + "="
	fileInfo["iv"] = fileInfo["iv"].(string) + "=="
	hashes := fileInfo["hashes"].(map[string]any)
	hashes["sha256"] = hashes["sha256"].(string) + "="

	assertDecryptsToPNG(t, ciphertext, fileInfo, want)
}

// A file that isn't the one its event names, such as a damaged download, must be refused rather
// than decrypted into noise
func TestDecryptAttachmentRefusesAFileWithTheWrongHash(t *testing.T) {
	ciphertext, fileInfo := encryptFile(t, testPNG(t), [32]byte{9}, [16]byte{8})
	ciphertext[len(ciphertext)/2] ^= 0x01

	if err := decryptAttachment(ciphertext, fileInfo); err == nil {
		t.Error("decryptAttachment() of a damaged file returned no error")
	}
}

func TestDecryptAttachmentRefusesMissingEncryptionInfo(t *testing.T) {
	if err := decryptAttachment([]byte("data"), nil); err == nil {
		t.Error("decryptAttachment() without encryption info returned no error")
	}
	if err := decryptAttachment([]byte("data"), map[string]any{"url": "mxc://example.com/file"}); err == nil {
		t.Error("decryptAttachment() with encryption info lacking a key returned no error")
	}
}
