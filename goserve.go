package main

import (
	"gopkg.in/v1/yaml"

	"flag"
	"io/ioutil"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

// ServerConfig represents a server configuration.
type ServerConfig struct {
	Listeners []Listener `yaml:"listeners"`
	Serves    []Serve    `yaml:"serve"`
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
			log.Printf(label + ": cert file `%s` does not exist", l.CertFile)
			ok = false
		}
		if _, err := os.Stat(l.KeyFile); os.IsNotExist(err) {
			log.Printf(label + ": key file `%s` does not exist", l.KeyFile)
			ok = false
		}
	} else {
		log.Printf(label + ": invalid protocol `%s`", l.Protocol)
		ok = false
	}
	return
}

// Serve represents a path that will be served.
type Serve struct {
	// Target specifies where files will be read from on the file system.
	Target string `yaml:"target"`
	// Path is the HTTP path that clients may use.
	Path string `yaml:"path"`
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
	if s.Target == "" {
		log.Println(label + ": no target path specified")
		ok = false
	}
	return
}

// Redirect represents a redirect from one path to another.
type Redirect struct {
	From  string `yaml:"from"`
	To    string `yaml:"to"`
	With  int    `yaml:"status"`
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

	// Setup serves
	for _, serve := range cfg.Serves {
		http.Handle(serve.Path, http.StripPrefix(serve.Path, http.FileServer(http.Dir(serve.Target))))
	}

	// Setup redirects
	for _, redirect := range cfg.Redirects {
		http.Handle(redirect.From, http.RedirectHandler(redirect.To, redirect.With))
	}

	// Start listeners
	for _, listener := range cfg.Listeners {
		if listener.Protocol == "http" {
			go func() {
				err := http.ListenAndServe(listener.Addr, nil)
				if err != nil {
					log.Fatalln(err)
				}
			}()
		} else if listener.Protocol == "https" {
			go func() {
				err := http.ListenAndServeTLS(listener.Addr, listener.CertFile, listener.KeyFile, nil)
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
