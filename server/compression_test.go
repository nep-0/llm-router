package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestCompressionMiddleware(t *testing.T) {
	// Create a simple handler that returns JSON
	handler := compressionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"hello world","data":"this is a test response that should be compressed"}`))
	})

	// Test 1: Request with br Accept-Encoding (Brotli)
	t.Run("WithBrotliAcceptEncoding", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		// Check that Content-Encoding is set to br
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "br" {
			t.Errorf("Expected Content-Encoding to be 'br', got '%s'", encoding)
		}

		// Decompress and verify content
		br := brotli.NewReader(resp.Body)
		body, err := io.ReadAll(br)
		if err != nil {
			t.Fatalf("Failed to read decompressed body: %v", err)
		}

		expected := `{"message":"hello world","data":"this is a test response that should be compressed"}`
		if string(body) != expected {
			t.Errorf("Expected body '%s', got '%s'", expected, string(body))
		}
	})

	// Test 2: Request with gzip Accept-Encoding
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

	// Test 3: Request without compression Accept-Encoding
	t.Run("WithoutCompressionAcceptEncoding", func(t *testing.T) {
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

	// Test 4: Request with multiple Accept-Encoding values - should prioritize br
	t.Run("WithMultipleAcceptEncodingsPrioritizeBrotli", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "deflate, gzip, br")
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		// Check that Content-Encoding is set to br (prioritized over gzip)
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "br" {
			t.Errorf("Expected Content-Encoding to be 'br' (prioritized), got '%s'", encoding)
		}

		// Decompress and verify content
		br := brotli.NewReader(resp.Body)
		body, err := io.ReadAll(br)
		if err != nil {
			t.Fatalf("Failed to read decompressed body: %v", err)
		}

		expected := `{"message":"hello world","data":"this is a test response that should be compressed"}`
		if string(body) != expected {
			t.Errorf("Expected body '%s', got '%s'", expected, string(body))
		}
	})

	// Test 5: Request with br listed after gzip - still prioritize br
	t.Run("WithBrotliAfterGzip", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip, br")
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		// Check that Content-Encoding is set to br (prioritized over gzip regardless of order)
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "br" {
			t.Errorf("Expected Content-Encoding to be 'br' (prioritized), got '%s'", encoding)
		}

		// Decompress and verify content
		br := brotli.NewReader(resp.Body)
		body, err := io.ReadAll(br)
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
	t.Run("GzipFlusher", func(t *testing.T) {
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
	})

	// Test Brotli flusher support
	t.Run("BrotliFlusher", func(t *testing.T) {
		handler := compressionMiddleware(func(w http.ResponseWriter, r *http.Request) {
			// Verify that w implements http.Flusher
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Error("brotli responseWriter does not implement http.Flusher")
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
		req.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		// Verify response was compressed
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "br" {
			t.Errorf("Expected Content-Encoding to be 'br', got '%s'", encoding)
		}

		// Decompress and verify content
		br := brotli.NewReader(resp.Body)
		body, err := io.ReadAll(br)
		if err != nil {
			t.Fatalf("Failed to read decompressed body: %v", err)
		}

		// Should contain all three chunks
		if count := strings.Count(string(body), "data: chunk"); count != 3 {
			t.Errorf("Expected 3 chunks, got %d", count)
		}
	})
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

	// Test with Brotli compression
	reqWithBrotli := httptest.NewRequest("GET", "/test", nil)
	reqWithBrotli.Header.Set("Accept-Encoding", "br")
	wWithBrotli := httptest.NewRecorder()
	handler(wWithBrotli, reqWithBrotli)

	// Test with Gzip compression
	reqWithGzip := httptest.NewRequest("GET", "/test", nil)
	reqWithGzip.Header.Set("Accept-Encoding", "gzip")
	wWithGzip := httptest.NewRecorder()
	handler(wWithGzip, reqWithGzip)

	// Test without compression
	reqWithoutCompression := httptest.NewRequest("GET", "/test", nil)
	wWithoutCompression := httptest.NewRecorder()
	handler(wWithoutCompression, reqWithoutCompression)

	brotliSize := wWithBrotli.Body.Len()
	gzipSize := wWithGzip.Body.Len()
	uncompressedSize := wWithoutCompression.Body.Len()

	// Both compressions should significantly reduce size
	if brotliSize >= uncompressedSize {
		t.Errorf("Brotli compressed size (%d) should be less than uncompressed size (%d)", brotliSize, uncompressedSize)
	}
	if gzipSize >= uncompressedSize {
		t.Errorf("Gzip compressed size (%d) should be less than uncompressed size (%d)", gzipSize, uncompressedSize)
	}

	brotliRatio := float64(brotliSize) / float64(uncompressedSize)
	gzipRatio := float64(gzipSize) / float64(uncompressedSize)

	t.Logf("Brotli compression ratio: %.2f%% (compressed: %d bytes, uncompressed: %d bytes)",
		brotliRatio*100, brotliSize, uncompressedSize)
	t.Logf("Gzip compression ratio: %.2f%% (compressed: %d bytes, uncompressed: %d bytes)",
		gzipRatio*100, gzipSize, uncompressedSize)

	// Verify Brotli compressed content is still valid
	respBr := wWithBrotli.Result()
	defer respBr.Body.Close()

	br := brotli.NewReader(respBr.Body)
	decompressedBr, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("Failed to read brotli decompressed body: %v", err)
	}

	if string(decompressedBr) != largeJSON {
		t.Error("Brotli decompressed content does not match original")
	}

	// Verify Gzip compressed content is still valid
	respGz := wWithGzip.Result()
	defer respGz.Body.Close()

	gz, err := gzip.NewReader(respGz.Body)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gz.Close()

	decompressedGz, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("Failed to read gzip decompressed body: %v", err)
	}

	if string(decompressedGz) != largeJSON {
		t.Error("Gzip decompressed content does not match original")
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

func TestBrotliWriterPooling(t *testing.T) {
	// Test that brotli writers are properly pooled and reused
	handler := compressionMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test response"))
	})

	// Make multiple requests
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()

		handler(w, req)

		resp := w.Result()
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "br" {
			t.Errorf("Request %d: Expected Content-Encoding to be 'br', got '%s'", i, encoding)
		}

		// Verify content is valid
		br := brotli.NewReader(resp.Body)
		body, err := io.ReadAll(br)
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

	b.Run("WithBrotliCompression", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Accept-Encoding", "br")
			w := httptest.NewRecorder()
			handler(w, req)
		}
	})

	b.Run("WithGzipCompression", func(b *testing.B) {
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
