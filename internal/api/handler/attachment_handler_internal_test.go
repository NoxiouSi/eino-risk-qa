package handler

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/config"
)

func multipartImageHeader(t *testing.T, filename string, data []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req, err := http.NewRequest(http.MethodPost, "/", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(int64(body.Len()+1024)))
	return req.MultipartForm.File["file"][0]
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, img))
	return data.Bytes()
}

func TestReadAndValidateImage_AcceptsDecodedPNG(t *testing.T) {
	data := tinyPNG(t)
	header := multipartImageHeader(t, "evidence.png", data)

	got, mimeType, err := readAndValidateImage(header, config.StorageConfig{MaxFileBytes: 1024, AllowedMIMETypes: []string{"image/png"}})

	require.NoError(t, err)
	assert.Equal(t, "image/png", mimeType)
	assert.Equal(t, data, got)
}

func TestReadAndValidateImage_RejectsExtensionMismatch(t *testing.T) {
	header := multipartImageHeader(t, "evidence.jpg", tinyPNG(t))

	_, _, err := readAndValidateImage(header, config.StorageConfig{MaxFileBytes: 1024, AllowedMIMETypes: []string{"image/png"}})

	assert.ErrorContains(t, err, "extension does not match")
}

func TestReadAndValidateImage_RejectsOversizeContent(t *testing.T) {
	header := multipartImageHeader(t, "evidence.png", tinyPNG(t))

	_, _, err := readAndValidateImage(header, config.StorageConfig{MaxFileBytes: 8, AllowedMIMETypes: []string{"image/png"}})

	assert.ErrorContains(t, err, "size exceeds")
}
