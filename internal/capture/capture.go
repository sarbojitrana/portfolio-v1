// Package capture screenshots a deployment and rejects blank or errored pages.
package capture

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	width       = 1280
	height      = 860
	cropHeight  = 800
	outputWidth = 820
	minVariance = 60.0
)

var browsers = []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "firefox"}

func browser() (string, error) {
	for _, b := range browsers {
		if p, err := exec.LookPath(b); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no headless browser found (tried %v)", browsers)
}

// Shot renders url and writes a downscaled JPEG to out.
func Shot(url, out string) error {
	bin, err := browser()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "shot")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	raw := filepath.Join(tmp, "shot.png")

	var args []string
	if filepath.Base(bin) == "firefox" {
		args = []string{"--headless", fmt.Sprintf("--window-size=%d,%d", width, height), "--screenshot", raw, url}
	} else {
		args = []string{"--headless", "--disable-gpu", "--hide-scrollbars",
			fmt.Sprintf("--window-size=%d,%d", width, height), "--screenshot=" + raw, url}
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "MOZ_HEADLESS=1")
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
		return fmt.Errorf("screenshot timed out: %s", url)
	}

	f, err := os.Open(raw)
	if err != nil {
		return fmt.Errorf("no screenshot produced for %s", url)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	img = cropTop(img, cropHeight)
	if v := variance(img); v < minVariance {
		return fmt.Errorf("page looks blank (variance %.1f): %s", v, url)
	}
	return writeJPEG(resize(img, outputWidth), out)
}

func cropTop(img image.Image, h int) image.Image {
	b := img.Bounds()
	if b.Dy() <= h {
		return img
	}
	type subImager interface {
		SubImage(image.Rectangle) image.Image
	}
	if s, ok := img.(subImager); ok {
		return s.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+h))
	}
	return img
}

func variance(img image.Image) float64 {
	b := img.Bounds()
	var sum, sumSq, n float64
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			r, g, bl, _ := img.At(x, y).RGBA()
			v := float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(bl>>8)*0.114
			sum += v
			sumSq += v * v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	mean := sum / n
	return math.Sqrt(math.Max(0, sumSq/n-mean*mean))
}

func resize(src image.Image, w int) image.Image {
	b := src.Bounds()
	if b.Dx() <= w {
		w = b.Dx()
	}
	scale := float64(b.Dx()) / float64(w)
	h := int(float64(b.Dy()) / scale)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	step := int(math.Max(1, scale))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bl, n uint32
			sx, sy := b.Min.X+int(float64(x)*scale), b.Min.Y+int(float64(y)*scale)
			for dy := 0; dy < step; dy++ {
				for dx := 0; dx < step; dx++ {
					px, py := sx+dx, sy+dy
					if px >= b.Max.X || py >= b.Max.Y {
						continue
					}
					cr, cg, cb, _ := src.At(px, py).RGBA()
					r, g, bl, n = r+cr>>8, g+cg>>8, bl+cb>>8, n+1
				}
			}
			if n == 0 {
				n = 1
			}
			dst.Set(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), 255})
		}
	}
	return dst
}

func writeJPEG(img image.Image, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 74})
}
