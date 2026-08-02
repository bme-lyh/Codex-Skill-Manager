package main

import (
	"bytes"
	"encoding/json"
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

type screenshotManifest struct {
	Frames []screenshotFrame
}

type screenshotFrame struct {
	File string
}

func main() {
	input := flag.String("input", filepath.FromSlash("docs/images/ui-screens"), "directory containing source PNG screenshots")
	output := flag.String("output", "", "output GIF path")
	locale := flag.String("locale", "en-US", "screenshot locale: en-US or zh-CN")
	manifestPath := flag.String("manifest", filepath.FromSlash("scripts/screenshot-frames.json"), "carousel frame manifest")
	width := flag.Int("width", 1440, "output width in pixels")
	height := flag.Int("height", 900, "output height in pixels")
	delay := flag.Int("delay", 220, "delay per frame in hundredths of a second")
	flag.Parse()

	if *locale != "en-US" && *locale != "zh-CN" {
		fail(fmt.Errorf("locale must be en-US or zh-CN"))
	}
	if *output == "" {
		*output = filepath.FromSlash("docs/images/ui-carousel." + *locale + ".gif")
	}
	if *width < 320 {
		fail(fmt.Errorf("width must be at least 320 pixels"))
	}
	if *height < 180 {
		fail(fmt.Errorf("height must be at least 180 pixels"))
	}
	if *delay < 20 {
		fail(fmt.Errorf("delay must be at least 20 hundredths of a second"))
	}

	manifest, err := readManifest(*manifestPath)
	if err != nil {
		fail(err)
	}
	if len(manifest.Frames) == 0 {
		fail(fmt.Errorf("manifest contains no frames"))
	}

	inputDirectory := filepath.Join(*input, *locale)
	animation := &gif.GIF{LoopCount: 0}
	for _, item := range manifest.Frames {
		sourcePath := filepath.Join(inputDirectory, item.File)
		source, err := readImage(sourcePath)
		if err != nil {
			fail(err)
		}

		frame := composeFrame(source, *width, *height)
		paletted, err := quantize(frame)
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

func readManifest(path string) (screenshotManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return screenshotManifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var manifest screenshotManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return screenshotManifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	for index, frame := range manifest.Frames {
		if filepath.Base(frame.File) != frame.File || frame.File == "" {
			return screenshotManifest{}, fmt.Errorf("frame %d has an invalid filename", index+1)
		}
	}
	return manifest, nil
}

func composeFrame(source image.Image, width, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(result, result.Bounds(), image.NewUniform(image.White), image.Point{}, draw.Src)
	bounds := source.Bounds()
	if bounds.Dx() <= width && bounds.Dy() <= height {
		offset := image.Pt((width-bounds.Dx())/2, (height-bounds.Dy())/2)
		target := image.Rectangle{Min: offset, Max: offset.Add(bounds.Size())}
		draw.Draw(result, target, source, bounds.Min, draw.Src)
		return result
	}
	scale := min(float64(width)/float64(bounds.Dx()), float64(height)/float64(bounds.Dy()))
	resizedWidth := max(1, int(float64(bounds.Dx())*scale))
	resizedHeight := max(1, int(float64(bounds.Dy())*scale))
	resized := resizeNearest(source, resizedWidth, resizedHeight)
	offset := image.Pt((width-resizedWidth)/2, (height-resizedHeight)/2)
	draw.Draw(result, image.Rectangle{Min: offset, Max: offset.Add(resized.Bounds().Size())}, resized, image.Point{}, draw.Src)
	return result
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
