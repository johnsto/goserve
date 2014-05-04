package main

import (
	"gopkg.in/v1/yaml"

	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var verbose bool
var cfg ServerConfig

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Increase verbosity")

	configPath := flag.String("config", "", "Path to configuration")
	checkConfig := flag.Bool("config.check", false, "Check config then quit")
	echoConfig := flag.Bool("config.echo", false, "Echo config then quit")

	indexes := flag.Bool("indexes", true, "Allow directory listing")

	httpEnabled := flag.Bool("http", true, "Enable HTTP listener")
	httpAddr := flag.String("http.addr", ":8080", "HTTP address")
	httpGzip := flag.Bool("http.gzip", true, "Enable HTTP gzip compression")

	httpsEnabled := flag.Bool("https", false, "Enable HTTPS listener")
	httpsAddr := flag.String("https.addr", ":8443", "HTTPS address")
	httpsGzip := flag.Bool("https.gzip", true, "Enable HTTPS gzip compression")
	httpsKey := flag.String("https.key", "", "Path to HTTPS key")
	httpsCert := flag.String("https.cert", "", "Path to HTTPS cert")

	flag.Parse()

	if *configPath == "" {
		if verbose {
			log.Println("Config file not specified; using arguments")
		}

		cfg.Listeners = []Listener{}

		if *httpEnabled {
			cfg.Listeners = append(cfg.Listeners, Listener{
				Protocol: "http",
				Addr:     *httpAddr,
				Gzip:     *httpGzip,
			})
		}
		if *httpsEnabled {
			cfg.Listeners = append(cfg.Listeners, Listener{
				Protocol: "https",
				Addr:     *httpsAddr,
				Gzip:     *httpsGzip,
				KeyFile:  *httpsKey,
				CertFile: *httpsCert,
			})
		}

		// Serve from first path given on cmdline
		target := flag.Arg(0)
		if target == "" {
			target = "."
		}

		cfg.Serves = []Serve{
			Serve{
				Path:    "/",
				Target:  target,
				Indexes: *indexes,
			},
		}
	} else {
		if verbose {
			log.Println("Config file specified; ignoring command line arguments")
		}

		var err error
		cfg, err = readServerConfig(*configPath)
		if err != nil {
			log.Fatalln("Couldn't load config:", err)
		}
	}

	cfg.sanitise()

	if *echoConfig {
		b, err := yaml.Marshal(cfg)
		if err != nil {
			log.Fatalln(err)
		}
		print(string(b))
	}

	if !cfg.check() {
		log.Fatalln("Invalid config. Exiting.")
	}

	if *checkConfig {
		log.Println("Config check passed.")
	}

	if *echoConfig || *checkConfig {
		os.Exit(0)
	}
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
	// Setup handlers
	mux := NewStaticServeMux()
	for _, e := range cfg.Errors {
		mux.HandleError(e.Status, e.handler())
	}
	for _, s := range cfg.Serves {
		mux.Handle(s.Path, s.handler())
	}
	for _, r := range cfg.Redirects {
		mux.Handle(r.From, r.handler())
	}

	// Start listeners
	for _, l := range cfg.Listeners {
		var h http.Handler = mux
		if len(l.Headers) > 0 {
			h = CustomHeadersHandler(h, l.Headers)
		}
		if l.Gzip {
			h = GzipHandler(h)
		}
		h = LogHandler(h)
		if l.Protocol == "http" {
			go func(l Listener) {
				if verbose {
					log.Printf("listening on HTTP %s\n", l.Addr)
				}
				err := http.ListenAndServe(l.Addr, h)
				if err != nil {
					log.Fatalln(err)
				}
			}(l)
		} else if l.Protocol == "https" {
			go func(l Listener) {
				if verbose {
					log.Printf(
						"listening on HTTPS %s (cert: %s, key: %s)\n",
						l.Addr, l.CertFile, l.KeyFile)
				}
				err := http.ListenAndServeTLS(l.Addr, l.CertFile, l.KeyFile, h)
				if err != nil {
					log.Fatalln(err)
				}
			}(l)
		} else {
			log.Printf("Unsupported protocol %s\n", l.Protocol)
		}
	}

	// Since all the listeners are running in separate gorotines, we have to
	// wait here for a termination signal.
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM)
	<-exit
	os.Exit(0)
}
