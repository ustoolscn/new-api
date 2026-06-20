package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadPlaygroundImagePostsMultipartAndReturnsURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer cooper" {
			t.Fatalf("Authorization = %q, want Bearer cooper", got)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Fatalf("Content-Type = %q, want multipart/form-data", r.Header.Get("Content-Type"))
		}

		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		file, header, err := r.FormFile("asset")
		if err != nil {
			t.Fatalf("FormFile(asset) error = %v", err)
		}
		defer file.Close()

		if header.Filename != "example.png" {
			t.Fatalf("filename = %q, want example.png", header.Filename)
		}
		if got := header.Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("part Content-Type = %q, want image/png", got)
		}
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if string(body) != "png-bytes" {
			t.Fatalf("uploaded body = %q, want png-bytes", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"url":"https://gallery.example/uploads/example.png","data":{"filename":"example.png"}}`))
	}))
	defer server.Close()

	image, err := UploadPlaygroundImageToHost(context.Background(), PlaygroundImageHostConfig{
		UploadURL:       server.URL,
		AuthHeader:      "Authorization",
		AuthValue:       "Bearer cooper",
		FieldName:       "asset",
		ResponseURLPath: "url",
	}, "example.png", "image/png", strings.NewReader("png-bytes"))
	if err != nil {
		t.Fatalf("UploadPlaygroundImageToHost() error = %v", err)
	}
	if image.URL != "https://gallery.example/uploads/example.png" {
		t.Fatalf("URL = %q, want gallery URL", image.URL)
	}
	if image.Filename != "example.png" {
		t.Fatalf("Filename = %q, want example.png", image.Filename)
	}
}

func TestUploadPlaygroundImageUsesConfiguredNestedURLPath(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"url":"https://gallery.example/nested.png"}}`))
	}))
	defer server.Close()

	image, err := UploadPlaygroundImageToHost(context.Background(), PlaygroundImageHostConfig{
		UploadURL:       server.URL,
		FieldName:       "file",
		ResponseURLPath: "data.url",
	}, "nested.png", "image/png", strings.NewReader("png-bytes"))
	if err != nil {
		t.Fatalf("UploadPlaygroundImageToHost() error = %v", err)
	}
	if image.URL != "https://gallery.example/nested.png" {
		t.Fatalf("URL = %q, want nested data URL", image.URL)
	}
}
