goserve
=======
A super dumb, super simple HTTP server designed for serving static files and requiring only rudimentary configuration.

[![Build Status](https://drone.io/github.com/johnsto/goserve/status.png)](https://drone.io/github.com/johnsto/goserve/latest)

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
  - path: /
    target: /var/wwwroot

errors:
  - status: 404
    target: /var/wwwroot/404.html

redirects:
  - from: files.myhost.com
    to: /files
  - from: /~files
    to: /files
    status: 302
```

Notes
-----
Goserve will serve up the `index.html` file of any directory that is requested. If `index.html` is not found, it will list the contents of the directory. If you don't want the contents of a directory to be listable, place an empty `index.html` file in the directory.

Implementation Notes
--------------------
Goserve  is little more than a configurable wrapper around Go's `http.ServeFile` handler, so it benefits from the caching/ETag behaviour of the default implementation.

To deal with errors, a custom `ResponseWriter` intercepts `WriteHeader` calls and attempts to serve up an appropriate error file (again, using `http.ServeFile`) for the status is known. Otherwise it falls through to the default implementation.
