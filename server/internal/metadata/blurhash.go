package metadata

import (
	"bytes"
	"errors"
	"image/color"
	"image/png"
	"math"
	"strings"
)

const base83 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~"

// blurhash encodes the tiny PNG media makes of an image as a BlurHash
// (blurha.sh). A BlurHash is a short string that clients paint while the
// image loads. It uses four components across the longer side and three
// across the shorter.
func blurhash(tiny []byte) (string, error) {
	img, err := png.Decode(bytes.NewReader(tiny))
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 || w*h > 64*256 {
		return "", errors.New("blurhash: not a tiny image")
	}
	nx, ny := 4, 3
	if h > w {
		nx, ny = 3, 4
	}

	linear := make([][3]float64, w*h)
	for y := range h {
		for x := range w {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			linear[y*w+x] = [3]float64{toLinear(c.R), toLinear(c.G), toLinear(c.B)}
		}
	}
	factors := make([][3]float64, 0, nx*ny)
	for j := range ny {
		for i := range nx {
			norm := 2.0
			if i == 0 && j == 0 {
				norm = 1
			}
			var f [3]float64
			for y := range h {
				for x := range w {
					basis := math.Cos(math.Pi*float64(i)*float64(x)/float64(w)) * math.Cos(math.Pi*float64(j)*float64(y)/float64(h))
					p := linear[y*w+x]
					f[0] += basis * p[0]
					f[1] += basis * p[1]
					f[2] += basis * p[2]
				}
			}
			scale := norm / float64(w*h)
			factors = append(factors, [3]float64{f[0] * scale, f[1] * scale, f[2] * scale})
		}
	}

	var sb strings.Builder
	encode83(&sb, (nx-1)+(ny-1)*9, 1)
	maxAC := 1.0
	if ac := factors[1:]; len(ac) > 0 {
		actual := 0.0
		for _, f := range ac {
			actual = max(actual, math.Abs(f[0]), math.Abs(f[1]), math.Abs(f[2]))
		}
		q := int(max(0, min(82, math.Floor(actual*166-0.5))))
		maxAC = float64(q+1) / 166
		encode83(&sb, q, 1)
	} else {
		encode83(&sb, 0, 1)
	}
	dc := factors[0]
	encode83(&sb, toSRGB(dc[0])<<16|toSRGB(dc[1])<<8|toSRGB(dc[2]), 4)
	for _, f := range factors[1:] {
		q := func(v float64) int {
			return int(max(0, min(18, math.Floor(signPow(v/maxAC, 0.5)*9+9.5))))
		}
		encode83(&sb, q(f[0])*19*19+q(f[1])*19+q(f[2]), 2)
	}
	return sb.String(), nil
}

func encode83(sb *strings.Builder, v, digits int) {
	for i := 1; i <= digits; i++ {
		d := v / int(math.Pow(83, float64(digits-i))) % 83
		sb.WriteByte(base83[d])
	}
}

func toLinear(v uint8) float64 {
	x := float64(v) / 255
	if x <= 0.04045 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

func toSRGB(v float64) int {
	v = max(0, min(1, v))
	if v <= 0.0031308 {
		return int(v*12.92*255 + 0.5)
	}
	return int((1.055*math.Pow(v, 1/2.4)-0.055)*255 + 0.5)
}

func signPow(v, exp float64) float64 {
	return math.Copysign(math.Pow(math.Abs(v), exp), v)
}
