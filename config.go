package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

// Headers represents a simplified HTTP header dict
type Headers map[string]string

// ServerConfig represents a server configuration.
type ServerConfig struct {
	Listeners []Listener `yaml:"listeners"`
	Serves    []Serve    `yaml:"serves"`
	Errors    []Error    `yaml:"errors,omitempty"`
	Redirects []Redirect `yaml:"redirects,omitempty"`
}

func (c ServerConfig) sanitise() {
	for i := range c.Listeners {
		c.Listeners[i].sanitise()
	}
	for i := range c.Serves {
		c.Serves[i].sanitise()
	}
	for i := range c.Redirects {
		c.Redirects[i].sanitise()
	}
	for i := range c.Errors {
		c.Errors[i].sanitise()
	}
}

func (c ServerConfig) check() (ok bool) {
	ok = true
	if len(c.Listeners) == 0 {
		log.Printf("No listeners defined!")
		ok = false
	}
	for i, l := range c.Listeners {
		ok = l.check(fmt.Sprintf("Listener #%d", i)) && ok
	}
	if len(c.Serves) == 0 {
		log.Printf("No serves defined!")
		ok = false
	}
	for i, s := range c.Serves {
		ok = s.check(fmt.Sprintf("Serve #%d", i)) && ok
	}
	for i, r := range c.Redirects {
		ok = r.check(fmt.Sprintf("Redirect #%d", i)) && ok
	}
	return
}

// Listener describes how connections are accepted and the protocol used.
type Listener struct {
	Protocol string  `yaml:"protocol"`
	Addr     string  `yaml:"addr"`
	CertFile string  `yaml:"cert,omitempty"`
	KeyFile  string  `yaml:"key,omitempty"`
	Headers  Headers `yaml:"headers,omitempty"` // custom headers
	Gzip     bool    `yaml:"gzip"`
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
	Target  string  `yaml:"target"`            // where files are stored on the file system
	Path    string  `yaml:"path"`              // HTTP path to serve files under
	Error   int     `yaml:"error,omitempty"`   // HTTP error to return (0=disabled)
	Indexes bool    `yaml:"indexes,omitempty"` // list directory contents
	Headers Headers `yaml:"headers,omitempty"` // custom headers
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
		log.Println(label + ": error specified with target path")
		ok = false
	}
	return
}

func (s Serve) handler() http.Handler {
	var h http.Handler
	if s.Error > 0 {
		errStatus := s.Error
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, http.StatusText(errStatus), errStatus)
		})
	} else if s.Indexes {
		h = http.FileServer(http.Dir(s.Target))
	} else {
		// Prevent listing of directories lacking an index.html file
		h = SuppressListingHandler(http.Dir(s.Target))
	}

	if len(s.Headers) > 0 {
		h = CustomHeadersHandler(h, s.Headers)
	}

	return http.StripPrefix(s.Path, h)
}

// Redirect represents a redirect from one path to another.
type Redirect struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	With int    `yaml:"status,omitempty"`
}

func (r *Redirect) sanitise() {
	if r.With == 0 {
		r.With = 301
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

func (r Redirect) handler() http.Handler {
	return http.RedirectHandler(r.To, r.With)
}

// Error represents what to do when a particular HTTP status is encountered.
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
		// Clear content-type as set by `http.Error` to force re-detection
		w.Header().Del("Content-Type")

		// Serve error page with a specific status code
		http.ServeFile(w, r, e.Target)
	})
}
