package api

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"testing"

	"github.com/makiuchi-d/gozxing"
	gozxingqr "github.com/makiuchi-d/gozxing/qrcode"
)

// A waiting device's QR code is a PNG that reads back as exactly the link
// it was given, at the address it reached the server by. Unknown and
// answered codes have none.
func TestDeviceCodeQR(t *testing.T) {
	e := newEnv(t, serving{BasePath: "/kivo"})
	dl := decode[map[string]any](t, e.do("POST", "/kivo/api/v1/device-codes", "", map[string]any{"name": "Living Room"}, "Kivo-Platform", "roku"))
	code, _ := dl["code"].(string)
	link, _ := dl["linkUrl"].(string)

	w := e.do("GET", "/kivo/api/v1/device-codes/"+code+"/qr", "", nil)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("qr: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"))
	}
	img, err := png.Decode(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode the PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() < 300 || b.Dx() != b.Dy() {
		t.Errorf("size %v, want a square at least 300 pixels across", b)
	}
	if got := readQR(t, img); got != link {
		t.Errorf("reads %q, want the link %q", got, link)
	}

	if w := e.do("GET", "/kivo/api/v1/device-codes/ZZZZZZ/qr", "", nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown code: %d", w.Code)
	}
	if w := e.do("POST", "/kivo/api/v1/device-codes/"+code+"/deny", e.nobodyToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("deny: %d", w.Code)
	}
	if w := e.do("GET", "/kivo/api/v1/device-codes/"+code+"/qr", "", nil); w.Code != http.StatusNotFound {
		t.Errorf("answered code: %d", w.Code)
	}
}

// readQR decodes the one QR code in img.
func readQR(t *testing.T, img image.Image) string {
	t.Helper()
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatalf("bitmap: %v", err)
	}
	res, err := gozxingqr.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		t.Fatalf("read the QR code: %v", err)
	}
	return res.GetText()
}
