goserve
=======
A super dumb, super simple HTTP server designed for serving static files and requiring only rudimentary configuration.

[![Build Status](https://drone.io/github.com/johnsto/goserve/status.png)](https://drone.io/github.com/johnsto/goserve/latest) [![Gobuild Download](http://gobuild.io/badge/github.com/johnsto/goserve/download.png)](http://gobuild.io/github.com/johnsto/goserve)

Features
--------
* ETag support
* Range handling
* HTTPS (TLS)
* Custom error pages
* Custom headers (v0.2)

Configuration
-------------
By default, `goserve` will serve the current directory on port 8080 when run without any parameters.

Alternatively, a configuration file can be specified using the `-config` parameter.

Config files are in the YAML format and have the following structure:

```
listeners:
  - protocol: http
    addr: ":80"
  - protocol: https
    addr: ":443"
    cert: cert.crt
    key: cert.key

serve:
  - path: /files/passwd
    error: 401
  - path: /files/
    target: /var/wwwfiles
    headers:
      Cache-Control: public, max-age=3600
  - path: /
    target: /var/wwwroot
    prevent-listing: true

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

Notes
-----
Goserve will serve up the `index.html` file of any directory that is requested. If `index.html` is not found, it will list the contents of the directory. If you don't want the contents of a directory to be listable, place an empty `index.html` file in the directory. Alternatively, specify `prevent-listing: true` on the serve to serve up a "403 Forbidden" error instead.

Implementation Notes
--------------------
Goserve is little more than a (admittedly rather hacky) configurable wrapper around Go's `http.ServeFile` handler, so it benefits from all the features of the default `FileServer` implementation (such as ETag support and range handling). Unfortunately, Go's `net/http` package doesn't expose quite as much control over the default `FileServer` implementation as one would like, so `goserve` uses a combination of wrapped handlers and `panic` intercepts to achieve the desired behaviour.

To deal with errors, a custom `ResponseWriter` intercepts `WriteHeader` calls and attempts to serve up an appropriate error file (again, using `http.ServeFile`) when the status is known. Otherwise it falls through to the default implementation.
