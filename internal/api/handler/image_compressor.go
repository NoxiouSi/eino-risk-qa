package handler

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxDecodedImagePixels = 40_000_000
	maxImageDimension     = 2560
	minImageDimension     = 64
)

func compressImageToJPEG(data []byte, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("compressed image size limit must be positive")
	}
	decoded, width, height, err := decodeImage(data)
	if err != nil {
		return nil, err
	}

	width, height = fitWithin(width, height, maxImageDimension)
	for {
		compressed, found, err := encodeWithinLimit(resizeOnWhite(decoded, width, height), maxBytes)
		if err != nil {
			return nil, err
		}
		if found {
			return compressed, nil
		}
		if width <= minImageDimension && height <= minImageDimension {
			return nil, fmt.Errorf("image cannot be compressed below size limit")
		}
		width = max(width*85/100, 1)
		height = max(height*85/100, 1)
	}
}

func decodeImage(data []byte) (image.Image, int, int, error) {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("invalid image content")
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || int64(width)*int64(height) > maxDecodedImagePixels {
		return nil, 0, 0, fmt.Errorf("image dimensions exceed limit")
	}
	return decoded, width, height, nil
}

func encodeWithinLimit(source image.Image, maxBytes int64) ([]byte, bool, error) {
	for quality := 88; quality >= 40; quality -= 8 {
		var output bytes.Buffer
		if err := jpeg.Encode(&output, source, &jpeg.Options{Quality: quality}); err != nil {
			return nil, false, fmt.Errorf("compress image failed: %w", err)
		}
		if int64(output.Len()) <= maxBytes {
			return output.Bytes(), true, nil
		}
	}
	return nil, false, nil
}

func fitWithin(width, height, limit int) (int, int) {
	if width <= limit && height <= limit {
		return width, height
	}
	if width >= height {
		return limit, max(height*limit/width, 1)
	}
	return max(width*limit/height, 1), limit
}

func resizeOnWhite(source image.Image, width, height int) *image.RGBA {
	destination := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(destination, destination.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, source.Bounds(), draw.Over, nil)
	return destination
}
