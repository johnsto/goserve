# goserve

*A plain HTTP server designed for serving static files with the most rudimentary of configuration.*

[![Build Status](https://drone.io/github.com/johnsto/goserve/status.png)](https://drone.io/github.com/johnsto/goserve/latest) [![Gobuild Download](http://gobuild.io/badge/github.com/johnsto/goserve/download.png)](http://gobuild.io/github.com/johnsto/goserve)

## Features

* ETag support
* Range handling
* HTTPS (TLS)
* Custom error pages
* Custom headers
* GZip compression

If you want anything more (or less!) than this, then you may want to consider writing your own - Go makes it [ridiculously simple](https://code.google.com/p/go-wiki/wiki/HttpStaticFiles) to serve static files out-of-the-box. For everything else, [Martini](http://martini.codegangsta.io) is worth a good look.

## Performance

In a completely arbitrary, unscientific and unreliable test on a machine where `python -m SimpleHTTPServer` achieved 46 reqs/sec and Node's `http-server` achieved 625 reqs/sec, `goserve` achieved 4716 reqs/sec.

## Installation

Either `go get github.com/johnsto/goserve`, or download a [binary from gobuild.io](http://gobuild.io/github.com/johnsto/goserve).

## Configuration

By default, `goserve` will serve the current directory via HTTP on port 8080 when run without any parameters. If a path argument is provided, `goserve` will serve from that directory instead.

Alternatively, a configuration file can be specified using the `-config` parameter for more advanced options.

### Command-line configuration

For cases where only one directory is being served, and there are no need for redirects or custom error handling, the command line is usually sufficient. For example, to serve the contents of `/var/www/` to the world over HTTPS (and only HTTPS), you might use:

```
goserve -http=false -https=true -https.cert=my.cert -https.key=my.key -https.addr="0.0.0.0:443" /var/www
```

The following parameters are supported:

```
  -config="": Path to configuration
  -config.check=false: Check config then quit
  -config.echo=false: Echo config then quit
  -http=true: Enable HTTP listener
  -http.addr=":8080": HTTP address
  -http.gzip=true: Enable HTTP gzip compression
  -https=false: Enable HTTPS listener
  -https.addr=":8443": HTTPS address
  -https.cert="": Path to HTTPS cert
  -https.gzip=true: Enable HTTPS gzip compression
  -https.key="": Path to HTTPS key
  -indexes=true: Allow directory listing
```

### File-based configuration

Config files expose additional functionality (such as error handlers and redirects) and have the following YAML structure:

```
listeners:
  - protocol: http
    addr: ":80"
    gzip: true
  - protocol: https
    addr: ":443"
    cert: cert.crt
    key: cert.key

serves:
  - path: /files/passwd
    error: 401
  - path: /files/
    target: /var/wwwfiles
    headers:
      Cache-Control: public, max-age=86400
  - path: /
    target: /var/wwwroot
    indexes: true # allow listing of directory contents

errors:
  - status: 404
    target: /var/wwwroot/notfound.html
  - status: 403
    target: /var/wwwroot/forbidden.html

redirects:
  - from: files.myhost.com
    to: /files
  - from: /~files
    to: /files
    status: 302
```

## Notes

Goserve will serve up the `index.html` file of any directory that is requested. If `index.html` is not found, it will list the contents of the directory. If you don't want the contents of a directory to be listable, place an empty `index.html` file in the directory. Alternatively, specify `prevent-listing: true` on the serve to serve up a "403 Forbidden" error instead.

### Implementation

Goserve is little more than a (admittedly rather hacky) configurable wrapper around Go's `http.ServeFile` handler, so it benefits from all the features of the default `FileServer` implementation (such as ETag support and range handling). Unfortunately, Go's `net/http` package doesn't expose quite as much control over the default `FileServer` implementation as one would like, so `goserve` uses a combination of wrapped handlers and `panic` intercepts to achieve the desired behaviour.

To deal with errors, a custom `ResponseWriter` intercepts `WriteHeader` calls and attempts to serve up an appropriate error file (again, using `http.ServeFile`) when the status is known. Otherwise it falls through to the default implementation.

Another hack is needed to prevent directory listing, which works in a similar fashion.
