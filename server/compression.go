package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
)

// gzipResponseWriter wraps http.ResponseWriter to provide gzip compression
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.Writer.Write(b)
}

// Implement http.Flusher interface for streaming support
func (w *gzipResponseWriter) Flush() {
	// Flush the gzip writer if it supports flushing
	if gw, ok := w.Writer.(*gzip.Writer); ok {
		gw.Flush()
	}
	// Flush the brotli writer if it supports flushing
	if bw, ok := w.Writer.(*brotli.Writer); ok {
		bw.Flush()
	}
	// Flush the underlying response writer if it supports flushing
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// gzipWriterPool pools gzip writers for reuse
var gzipWriterPool = sync.Pool{
	New: func() any {
		return gzip.NewWriter(io.Discard)
	},
}

// brotliWriterPool pools brotli writers for reuse
var brotliWriterPool = sync.Pool{
	New: func() any {
		return brotli.NewWriter(io.Discard)
	},
}

// compressionMiddleware wraps an http.Handler to add compression support
// Prioritizes Brotli (br) over gzip
func compressionMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acceptEncoding := r.Header.Get("Accept-Encoding")

		// Check for Brotli support first (prioritize br over gzip)
		if strings.Contains(acceptEncoding, "br") {
			// Get a brotli writer from the pool
			br := brotliWriterPool.Get().(*brotli.Writer)
			defer brotliWriterPool.Put(br)

			br.Reset(w)
			defer br.Close()

			// Set the content encoding header
			w.Header().Set("Content-Encoding", "br")
			// Remove Content-Length header since we're compressing
			w.Header().Del("Content-Length")

			// Wrap the response writer
			brw := &gzipResponseWriter{
				Writer:         br,
				ResponseWriter: w,
			}

			next(brw, r)
			return
		}

		// Check if client accepts gzip encoding
		if strings.Contains(acceptEncoding, "gzip") {
			// Get a gzip writer from the pool
			gz := gzipWriterPool.Get().(*gzip.Writer)
			defer gzipWriterPool.Put(gz)

			gz.Reset(w)
			defer gz.Close()

			// Set the content encoding header
			w.Header().Set("Content-Encoding", "gzip")
			// Remove Content-Length header since we're compressing
			w.Header().Del("Content-Length")

			// Wrap the response writer
			gzw := &gzipResponseWriter{
				Writer:         gz,
				ResponseWriter: w,
			}

			next(gzw, r)
			return
		}

		// No compression
		next(w, r)
	}
}
