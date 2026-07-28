package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
)

var frames = []string{
	"overview.png",
	"skills.png",
	"skills-batch.png",
	"groups.png",
	"updates.png",
	"security-summary.png",
	"security-clusters.png",
	"install-preview.png",
	"history.png",
	"quarantine.png",
	"reports.png",
	"settings.png",
}

func main() {
	input := flag.String("input", filepath.FromSlash("docs/images/ui-screens"), "directory containing source PNG screenshots")
	output := flag.String("output", filepath.FromSlash("docs/images/ui-carousel.gif"), "output GIF path")
	width := flag.Int("width", 1000, "output width in pixels")
	delay := flag.Int("delay", 220, "delay per frame in hundredths of a second")
	flag.Parse()

	if *width < 320 {
		fail(fmt.Errorf("width must be at least 320 pixels"))
	}
	if *delay < 20 {
		fail(fmt.Errorf("delay must be at least 20 hundredths of a second"))
	}

	animation := &gif.GIF{LoopCount: 0}
	for _, name := range frames {
		sourcePath := filepath.Join(*input, name)
		source, err := readImage(sourcePath)
		if err != nil {
			fail(err)
		}

		height := source.Bounds().Dy() * *width / source.Bounds().Dx()
		resized := resizeNearest(source, *width, height)
		paletted, err := quantize(resized)
		if err != nil {
			fail(fmt.Errorf("quantize %s: %w", sourcePath, err))
		}

		animation.Image = append(animation.Image, paletted)
		animation.Delay = append(animation.Delay, *delay)
		animation.Disposal = append(animation.Disposal, gif.DisposalNone)
	}

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fail(fmt.Errorf("create output directory: %w", err))
	}
	file, err := os.Create(*output)
	if err != nil {
		fail(fmt.Errorf("create output: %w", err))
	}
	defer file.Close()

	if err := gif.EncodeAll(file, animation); err != nil {
		fail(fmt.Errorf("encode animation: %w", err))
	}
	fmt.Printf("Screenshot carousel created: %s\n", *output)
}

func readImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	decoded, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return decoded, nil
}

func resizeNearest(source image.Image, width, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + y*bounds.Dy()/height
		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + x*bounds.Dx()/width
			result.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return result
}

func quantize(source image.Image) (*image.Paletted, error) {
	var buffer bytes.Buffer
	if err := gif.Encode(&buffer, source, &gif.Options{
		NumColors: 256,
		Drawer:    draw.FloydSteinberg,
	}); err != nil {
		return nil, err
	}
	decoded, err := gif.Decode(&buffer)
	if err != nil {
		return nil, err
	}
	paletted, ok := decoded.(*image.Paletted)
	if !ok {
		return nil, fmt.Errorf("GIF decoder returned %T", decoded)
	}
	return paletted, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
