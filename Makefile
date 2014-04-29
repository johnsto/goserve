build: goserve

goserve: *.go
	go build -o $@ $^

fmt: *.go
	go fmt $^
