package main

import (
	"gopkg.in/v1/yaml"

	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"strings"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

// Headers represents a simplified HTTP header dict
type Headers map[string]string

// ServerConfig represents a server configuration.
type ServerConfig struct {
	Listeners []Listener `yaml:"listeners"`
	Serves    []Serve    `yaml:"serve"`
	Errors    []Error    `yaml:"errors"`
	Redirects []Redirect `yaml:"redirects"`
}

// DefaultServerConfig creates a basic server config on a non-privileged port
// that serves up files from the CWD to the root path over HTTP.
func DefaultServerConfig() ServerConfig {
	c := ServerConfig{}
	c.Listeners = []Listener{
		Listener{Protocol: "http", Addr: ":8080"},
	}
	c.Serves = []Serve{
		Serve{Path: "/", Target: "."},
	}
	return c
}

func (c ServerConfig) sanitise() {
	for _, l := range c.Listeners {
		l.sanitise()
	}
	for _, s := range c.Serves {
		s.sanitise()
	}
	for _, r := range c.Redirects {
		r.sanitise()
	}
	for _, e := range c.Errors {
		e.sanitise()
	}
}

func (c ServerConfig) check() (ok bool) {
	ok = true
	if len(c.Listeners) == 0 {
		log.Printf("No listeners defined!")
		ok = false
	}
	for i, l := range c.Listeners {
		ok = l.check(fmt.Sprintf("Listener #%d:", i)) && ok
	}
	if len(c.Serves) == 0 {
		log.Printf("No serves defined!")
		ok = false
	}
	for i, s := range c.Serves {
		ok = s.check(fmt.Sprintf("Serve #%d:", i)) && ok
	}
	for i, r := range c.Redirects {
		ok = r.check(fmt.Sprintf("Redirect #%d:", i)) && ok
	}
	return
}

// Listener describes how connections are accepted and the protocol used.
type Listener struct {
	Protocol string `yaml:"protocol"`
	Addr     string `yaml:"addr"`
	CertFile string `yaml:"cert"`
	KeyFile  string `yaml:"key"`
	Headers		   Headers `yaml:"headers"` // custom headers
	Gzip bool `yaml:"gzip"`
}

func (l *Listener) sanitise() {
	if l.Protocol == "" {
		l.Protocol = "http"
	}
	if l.Addr == "" {
		l.Addr = ":http"
	}
}

func (l *Listener) check(label string) (ok bool) {
	ok = true
	if l.Protocol == "http" {
		if l.CertFile != "" || l.KeyFile != "" {
			log.Printf(label + ": certificate supplied for non-HTTPS listener")
			ok = false
		}
	} else if l.Protocol == "https" {
		if _, err := os.Stat(l.CertFile); os.IsNotExist(err) {
			log.Printf(label+": cert file `%s` does not exist", l.CertFile)
			ok = false
		}
		if _, err := os.Stat(l.KeyFile); os.IsNotExist(err) {
			log.Printf(label+": key file `%s` does not exist", l.KeyFile)
			ok = false
		}
	} else {
		log.Printf(label+": invalid protocol `%s`", l.Protocol)
		ok = false
	}
	return
}

// Serve represents a path that will be served.
type Serve struct {
	Target         string `yaml:"target"`          // where files are stored on the file system
	Path           string `yaml:"path"`            // HTTP path to serve files under
	Error          int    `yaml:"error"`           // HTTP error to return (0=disabled)
	PreventListing bool   `yaml:"prevent-listing"` // prevent file listing
	Headers		   Headers `yaml:"headers"` // custom headers
}

func (s *Serve) sanitise() {
	if s.Path == "" {
		s.Path = "/"
	}
}

func (s Serve) check(label string) (ok bool) {
	ok = true
	if s.Path == "" {
		log.Println(label + ": no path specified")
		ok = false
	}
	if s.Error == 0 && s.Target == "" {
		log.Println(label + ": no target path specified")
		ok = false
	}
	if s.Error != 0 && s.Target != "" {
		log.Println(label + ": error specificied with target path")
		ok = false
	}
	return
}

// Redirect represents a redirect from one path to another.
type Redirect struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	With int    `yaml:"status"`
}

func (r *Redirect) sanitise() {
	if r.With == 0 {
		r.With = 301
		log.Printf("Defaulting status code %d for redirect %s\n", r.With, r.From)
	}
}

func (r Redirect) check(label string) (ok bool) {
	if r.From == "" {
		log.Printf(label + ": no `from` path")
		ok = false
	}
	if r.To == "" {
		log.Printf(label + ": no `to` path")
		ok = false
	}
	return true
}

// Error represents what to do when a particuar HTTP status is encountered.
type Error struct {
	Status int    `yaml:"status"`
	Target string `yaml:"target"`
}

func (e *Error) sanitise() {
}

func (e Error) check() (ok bool) {
	return true
}

func (e Error) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clear any existing headers
		for k := range w.Header() {
			w.Header().Del(k)
		}
		w.WriteHeader(e.Status)

		// Forbid serveFile from writing a new header by wrapping RW with
		// nil version.
		http.ServeFile(&NoHeaderResponseWriter{ResponseWriter: w}, r, e.Target)
	})
}

// NoHeaderResponseWriter ignores WriteHeader calls. Useful when you know
// the HTTP status has already been emitted.
type NoHeaderResponseWriter struct {
	http.ResponseWriter
}

// WriteHeader does nothing. Useful when you know a header has already been
// written.
func (h *NoHeaderResponseWriter) WriteHeader(status int) {}

// ErrorHandlingResponseWriter intercepts known error codes and responds
// with a different Handler.
type ErrorHandlingResponseWriter struct {
	http.ResponseWriter
	r             *http.Request
	ErrorHandlers map[int]http.Handler
}

// WriteHeader hijacks the response if the error code is known to it, and
// responds using a predetermined Handler if possible.
func (h *ErrorHandlingResponseWriter) WriteHeader(status int) {
	handler := h.ErrorHandlers[status]
	if handler != nil {
		handler.ServeHTTP(h.ResponseWriter, h.r)
		panic(h)
	} else {
		h.ResponseWriter.WriteHeader(status)
	}
}

// ErrorHandler wraps a Handler such that any HTTP errors can be dealt with
// using a custom Handler.
func ErrorHandler(handler http.Handler, errorHandlers map[int]http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ehrw := &ErrorHandlingResponseWriter{ResponseWriter: w, r: r, ErrorHandlers: errorHandlers}

		// If error was intercepted, originating call would have been panic'd.
		// Recover here once error has been dealt with.
		defer func() {
			if p := recover(); p != nil {
				if p == ehrw {
					return
				}
				if p == 1 {
					http.Error(w, "way", 404)
					return
				}
				panic(p)
			}
		}()

		handler.ServeHTTP(ehrw, r)
	})
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
	return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
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

var verbose bool
var configPath string
var checkConfig bool

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Verbose")
	flag.StringVar(&configPath, "config", "", "Path to configuration")
	flag.BoolVar(&checkConfig, "check", false, "Only check config")

	flag.Parse()
}

func readServerConfig(filename string) (cfg ServerConfig, err error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(data, &cfg)
	return
}

func main() {
	cfg := DefaultServerConfig()
	if configPath != "" {
		var err error
		cfg, err = readServerConfig(configPath)
		if err != nil {
			log.Fatalln("Couldn't load config:", err)
		}
	}
	cfg.sanitise()

	if !cfg.check() {
		log.Fatalln("Invalid config.")
	}
	if checkConfig {
		log.Println("Config check passed")
		os.Exit(0)
	}

	errorHandlers := make(map[int]http.Handler)
	for _, e := range cfg.Errors {
		errorHandlers[e.Status] = e.handler()
	}

	// Setup serves
	for _, serve := range cfg.Serves {
		var h http.Handler
		if serve.Error > 0 {
			errStatus := serve.Error
			h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, http.StatusText(errStatus), errStatus)
			})
		} else if serve.PreventListing {
			// Prevent listing of directories lacking an index.html file
			target := serve.Target
			h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				d := &PreventListingDir{http.Dir(target)}
				fs := http.FileServer(d)
				defer func() {
					if p := recover(); p != nil {
						if p == d {
							http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
							return
						}
						panic(p)
					}
				}()
				fs.ServeHTTP(w, r)
			})
		} else {
			h = http.FileServer(http.Dir(serve.Target))
		}
		if len(serve.Headers) > 0 {
			h = CustomHeadersHandler(h, serve.Headers)
		}
		eh := ErrorHandler(h, errorHandlers)
		http.Handle(serve.Path, http.StripPrefix(serve.Path, eh))
	}

	// Setup redirects
	for _, redirect := range cfg.Redirects {
		http.Handle(redirect.From, http.RedirectHandler(redirect.To, redirect.With))
	}

	// Start listeners
	for _, listener := range cfg.Listeners {
		var h http.Handler = http.DefaultServeMux
		if listener.Gzip {
			h = GzipHandler(h)
		}
		if len(listener.Headers) > 0 {
			h = CustomHeadersHandler(h, listener.Headers)
		}
		if listener.Protocol == "http" {
			go func() {
				err := http.ListenAndServe(listener.Addr, h)
				if err != nil {
					log.Fatalln(err)
				}
			}()
		} else if listener.Protocol == "https" {
			go func() {
				err := http.ListenAndServeTLS(listener.Addr, listener.CertFile, listener.KeyFile, h)
				if err != nil {
					log.Fatalln(err)
				}
			}()
		} else {
			log.Printf("Unsupported protocol %s\n", listener.Protocol)
		}
		log.Printf("listening on %s (%s)\n", listener.Addr, listener.Protocol)
	}

	// Since all the listeners are running in separate gorotines, we have to
	// wait here for a termination signal.
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM)
	<-exit
	os.Exit(0)
}
