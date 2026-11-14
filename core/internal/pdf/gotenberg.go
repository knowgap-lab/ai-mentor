package pdf

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

// ErrUnavailable — Gotenberg не задан, не ответил или не вернул PDF.
var ErrUnavailable = errors.New("pdf unavailable")

const gotenbergEnv = "PDF_GOTENBERG_URL"

var renderClient = &http.Client{Timeout: 60 * time.Second}

// Render отправляет HTML в Gotenberg и возвращает байты PDF.
// Адрес читается из PDF_GOTENBERG_URL при каждом вызове.
func Render(ctx context.Context, html string) ([]byte, error) {
	base := strings.TrimSpace(os.Getenv(gotenbergEnv))
	if base == "" {
		return nil, ErrUnavailable
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, ErrUnavailable
	}
	if _, err := part.Write([]byte(html)); err != nil {
		return nil, ErrUnavailable
	}
	if err := writer.Close(); err != nil {
		return nil, ErrUnavailable
	}
	endpoint := strings.TrimRight(base, "/") + "/forms/chromium/convert/html"
	//nolint:gosec // G704: адрес из PDF_GOTENBERG_URL, не из запроса.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := renderClient.Do(req) //nolint:gosec // G704: тот же адрес PDF_GOTENBERG_URL.
	if err != nil {
		return nil, ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil || res.StatusCode != http.StatusOK || !bytes.HasPrefix(payload, []byte("%PDF-")) {
		return nil, ErrUnavailable
	}
	return payload, nil
}
