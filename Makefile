all: clean generate native raspi

raspi:
	env GOOS=linux GOARCH=arm GOARM=5 go build -v -ldflags "-X main.buildDate=$(shell date -u +%FT%TZ) -X main.buildVersion=$(shell git describe --dirty)" -o vogod_raspi ./cmd/vogod/main.go

native:
	go build -v -ldflags "-X main.buildDate=$(shell date -u +%FT%TZ) -X main.buildVersion=$(shell git describe --dirty)" -o vogod ./cmd/vogod/main.go

generate:
	go generate ./...

clean:
	rm -f ./vogod ./vogod_*
