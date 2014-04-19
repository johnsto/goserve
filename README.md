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
