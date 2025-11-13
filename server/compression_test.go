package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompressionMiddleware(t *testing.T) {
	// Create a simple handler that returns JSON
	handler := compressionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"hello world","data":"this is a test response that should be compressed"}`))
	})

	// Test 1: Request with gzip Accept-Encoding
	t.Run("WithGzipAcceptEncoding", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		// Check that Content-Encoding is set to gzip
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "gzip" {
			t.Errorf("Expected Content-Encoding to be 'gzip', got '%s'", encoding)
		}

		// Decompress and verify content
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatalf("Failed to create gzip reader: %v", err)
		}
		defer gz.Close()

		body, err := io.ReadAll(gz)
		if err != nil {
			t.Fatalf("Failed to read decompressed body: %v", err)
		}

		expected := `{"message":"hello world","data":"this is a test response that should be compressed"}`
		if string(body) != expected {
			t.Errorf("Expected body '%s', got '%s'", expected, string(body))
		}
	})

	// Test 2: Request without gzip Accept-Encoding
	t.Run("WithoutGzipAcceptEncoding", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		// Check that Content-Encoding is NOT set
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "" {
			t.Errorf("Expected Content-Encoding to be empty, got '%s'", encoding)
		}

		// Read uncompressed content
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read body: %v", err)
		}

		expected := `{"message":"hello world","data":"this is a test response that should be compressed"}`
		if string(body) != expected {
			t.Errorf("Expected body '%s', got '%s'", expected, string(body))
		}
	})

	// Test 3: Request with multiple Accept-Encoding values
	t.Run("WithMultipleAcceptEncodings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "deflate, gzip, br")
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		// Check that Content-Encoding is set to gzip
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "gzip" {
			t.Errorf("Expected Content-Encoding to be 'gzip', got '%s'", encoding)
		}

		// Decompress and verify content
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatalf("Failed to create gzip reader: %v", err)
		}
		defer gz.Close()

		body, err := io.ReadAll(gz)
		if err != nil {
			t.Fatalf("Failed to read decompressed body: %v", err)
		}

		expected := `{"message":"hello world","data":"this is a test response that should be compressed"}`
		if string(body) != expected {
			t.Errorf("Expected body '%s', got '%s'", expected, string(body))
		}
	})
}

func TestGzipResponseWriterFlusher(t *testing.T) {
	// Test that the gzipResponseWriter implements http.Flusher for streaming
	handler := compressionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Verify that w implements http.Flusher
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("gzipResponseWriter does not implement http.Flusher")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Write and flush multiple times (simulating SSE streaming)
		for i := 0; i < 3; i++ {
			w.Write([]byte("data: chunk\n\n"))
			flusher.Flush()
		}
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Verify response was compressed
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "gzip" {
		t.Errorf("Expected Content-Encoding to be 'gzip', got '%s'", encoding)
	}

	// Decompress and verify content
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gz.Close()

	body, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("Failed to read decompressed body: %v", err)
	}

	// Should contain all three chunks
	if count := strings.Count(string(body), "data: chunk"); count != 3 {
		t.Errorf("Expected 3 chunks, got %d", count)
	}
}

func TestCompressionSavesSpace(t *testing.T) {
	// Create a large JSON response to compress
	largeJSON := `{"users":[`
	for i := 0; i < 100; i++ {
		if i > 0 {
			largeJSON += ","
		}
		largeJSON += `{"id":` + string(rune('0'+i%10)) + `,"name":"User Name","email":"user@example.com","description":"This is a long description that will compress well"}`
	}
	largeJSON += `]}`

	handler := compressionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(largeJSON))
	})

	// Test with compression
	reqWithGzip := httptest.NewRequest("GET", "/test", nil)
	reqWithGzip.Header.Set("Accept-Encoding", "gzip")
	wWithGzip := httptest.NewRecorder()
	handler(wWithGzip, reqWithGzip)

	// Test without compression
	reqWithoutGzip := httptest.NewRequest("GET", "/test", nil)
	wWithoutGzip := httptest.NewRecorder()
	handler(wWithoutGzip, reqWithoutGzip)

	compressedSize := wWithGzip.Body.Len()
	uncompressedSize := wWithoutGzip.Body.Len()

	// Compression should significantly reduce size
	if compressedSize >= uncompressedSize {
		t.Errorf("Compressed size (%d) should be less than uncompressed size (%d)", compressedSize, uncompressedSize)
	}

	compressionRatio := float64(compressedSize) / float64(uncompressedSize)
	t.Logf("Compression ratio: %.2f%% (compressed: %d bytes, uncompressed: %d bytes)",
		compressionRatio*100, compressedSize, uncompressedSize)

	// Verify compressed content is still valid
	resp := wWithGzip.Result()
	defer resp.Body.Close()

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gz.Close()

	decompressed, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("Failed to read decompressed body: %v", err)
	}

	if string(decompressed) != largeJSON {
		t.Error("Decompressed content does not match original")
	}
}

func TestGzipWriterPooling(t *testing.T) {
	// Test that gzip writers are properly pooled and reused
	handler := compressionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test response"))
	})

	// Make multiple requests
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "gzip" {
			t.Errorf("Request %d: Expected Content-Encoding to be 'gzip', got '%s'", i, encoding)
		}

		// Verify content is valid
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatalf("Request %d: Failed to create gzip reader: %v", i, err)
		}

		body, err := io.ReadAll(gz)
		gz.Close()
		resp.Body.Close()

		if err != nil {
			t.Fatalf("Request %d: Failed to read body: %v", i, err)
		}

		if string(body) != "test response" {
			t.Errorf("Request %d: Expected 'test response', got '%s'", i, string(body))
		}
	}
}

func BenchmarkCompressionMiddleware(b *testing.B) {
	largeJSON := bytes.Repeat([]byte(`{"key":"value","description":"some text"}`), 100)

	handler := compressionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Write(largeJSON)
	})

	b.Run("WithCompression", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			w := httptest.NewRecorder()
			handler(w, req)
		}
	})

	b.Run("WithoutCompression", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			handler(w, req)
		}
	})
}
