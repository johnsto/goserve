package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// StaticServeMux wraps ServeMux but allows for the interception of errors.
type StaticServeMux struct {
	*http.ServeMux
	errors map[int]http.Handler
}

// NewStaticServeMux allocates and returns a new StaticServeMux
func NewStaticServeMux() *StaticServeMux {
	return &StaticServeMux{
		ServeMux: http.NewServeMux(),
		errors:   make(map[int]http.Handler),
	}
}

// HandleError registers a handler for the given response code.
func (s *StaticServeMux) HandleError(status int, handler http.Handler) {
	if s.errors[status] != nil {
		panic("Handler for error already registered")
	}
	s.errors[status] = handler
}

func (s StaticServeMux) intercept(status int, w http.ResponseWriter, req *http.Request) bool {
	// Get error handler if there is one
	if h, f := s.errors[status]; f {
		h.ServeHTTP(statusResponseWriter{w, status}, req)
		return true
	}
	// Ignore non-error status codes
	if status < 400 {
		return false
	}
	http.Error(w, http.StatusText(status), status)
	return true
}

func (s *StaticServeMux) interceptHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		irw := &InterceptResponseWriter{
			ResponseWriter: w,
			r:              r,
			m:              s,
		}

		// If intercept occurred, originating call would have been panic'd.
		// Recover here once error has been dealt with.
		defer func() {
			if p := recover(); p != nil {
				if p == irw {
					return
				}
				panic(p)
			}
		}()

		handler.ServeHTTP(irw, r)
	})
}

func (s *StaticServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "*" {
		if r.ProtoAtLeast(1, 1) {
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h, _ := s.Handler(r)
	h = s.interceptHandler(h)
	h.ServeHTTP(w, r)
}

type InterceptResponseWriter struct {
	http.ResponseWriter
	r *http.Request
	m *StaticServeMux
}

func (h *InterceptResponseWriter) WriteHeader(status int) {
	if h.m.intercept(status, h.ResponseWriter, h.r) {
		panic(h)
	} else {
		h.ResponseWriter.WriteHeader(status)
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	Status int
}

func (h statusResponseWriter) WriteHeader(status int) {
	if h.Status < 0 {
		return
	}
	if h.Status > 0 {
		h.ResponseWriter.WriteHeader(h.Status)
		return
	}
	h.ResponseWriter.WriteHeader(status)
}

// PreventListingDir panics whenever a file open fails, allowing index
// requests to be intercepted.
type PreventListingDir struct {
	http.Dir
}

// Open panics whenever opening a file fails.
func (dir *PreventListingDir) Open(name string) (f http.File, err error) {
	f, err = dir.Dir.Open(name)
	if f == nil {
		panic(dir)
	}
	return
}

// SuppressListingHandler returns a FileServer handler that does not permit
// the listing of files.
func SuppressListingHandler(dir http.Dir) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := &PreventListingDir{dir}
		h := http.FileServer(d)
		defer func() {
			if p := recover(); p != nil {
				if p == d {
					http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
					return
				}
				panic(p)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

// CustomHeadersHandler creates a new handler that includes the provided
// headers in each response.
func CustomHeadersHandler(h http.Handler, headers Headers) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wh := w.Header()
		for k, v := range headers {
			if wh.Get(k) == "" {
				wh.Set(k, v)
			}
		}
		h.ServeHTTP(w, r)
	})
}

// GzipResponseWriter gzips content written to it
type GzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	gotContentType bool
}

func (w *GzipResponseWriter) Write(b []byte) (int, error) {
	if !w.gotContentType {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.gotContentType = true
	}
	return w.Writer.Write(b)
}

// GzipHandler gzips the HTTP response if supported by the client. Based on
// the implementation of `go.httpgzip`
func GzipHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve normally to clients that don't express gzip support
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		h.ServeHTTP(&GzipResponseWriter{Writer: gz, ResponseWriter: w}, r)
	})
}

func LogHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := NewLoggingResponseWriter(w)
		h.ServeHTTP(rw, r)
		rw.log(r)
	})
}

type LoggingResponseWriter struct {
	http.ResponseWriter
	status *int
	out    io.Writer
}

func NewLoggingResponseWriter(w http.ResponseWriter) LoggingResponseWriter {
	return LoggingResponseWriter{
		ResponseWriter: w,
		status:         new(int),
		out:            os.Stdout,
	}
}

func (w LoggingResponseWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
	*w.status = status
}

func (w LoggingResponseWriter) log(req *http.Request) {
	fmt.Printf(
		"%s - %s - %s - %s - %d\n",
		time.Now().Format(time.RFC3339),
		req.RemoteAddr,
		req.Host,
		strconv.Quote(req.Method+" "+req.RequestURI),
		*w.status)
}
