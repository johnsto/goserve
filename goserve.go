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

var cfg ServerConfig

func init() {
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
	for _, serve := range cfg.Serves {
		mux.Handle(serve.Path, serve.handler())
	}
	for _, redirect := range cfg.Redirects {
		mux.Handle(redirect.From, redirect.handler())
	}

	// Start listeners
	for i := range cfg.Listeners {
		listener := cfg.Listeners[i]

		var h http.Handler = mux
		if len(listener.Headers) > 0 {
			h = CustomHeadersHandler(h, listener.Headers)
		}
		if listener.Gzip {
			h = GzipHandler(h)
		}
		if listener.Protocol == "http" {
			go func() {
				log.Printf("listening on HTTP %s\n", listener.Addr)
				err := http.ListenAndServe(listener.Addr, h)
				if err != nil {
					log.Fatalln(err)
				}
			}()
		} else if listener.Protocol == "https" {
			go func() {
				log.Printf("listening on HTTPS %s\n", listener.Addr)
				err := http.ListenAndServeTLS(listener.Addr, listener.CertFile, listener.KeyFile, h)
				if err != nil {
					log.Fatalln(err)
				}
			}()
		} else {
			log.Printf("Unsupported protocol %s\n", listener.Protocol)
		}
	}

	// Since all the listeners are running in separate gorotines, we have to
	// wait here for a termination signal.
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM)
	<-exit
	os.Exit(0)
}
