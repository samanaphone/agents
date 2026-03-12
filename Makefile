
SRC_FILES=$(wildcard ./pkg/*/*.go)
TMPL_FILES=$(wildcard ./templates/*)
STATIC_FILES=$(wildcard ./static/*)
VERSION=0.0.2

DIST_PACKAGES=sammagents-linux-aarch64-$(VERSION).tgz sammagents-linux-amd64-$(VERSION).tgz

all: build/main

build:
	mkdir -p build

build/main-linux-amd64: ./cmd/dashboard/main.go build $(SRC_FILES)
	GOOS=linux GOARCH=amd64 go build -o $@ $<

build/main-linux-aarch64: ./cmd/dashboard/main.go build $(SRC_FILES)
	GOOS=linux GOARCH=arm64 go build -o $@ $<

dist: $(DIST_PACKAGES)

sammagents-linux-aarch64-$(VERSION).tgz: build/main-linux-aarch64 $(TMPL_FILES) $(STATIC_FILES)
	mkdir tempaarch64
	cp -R static templates .env.example docker/* tempaarch64
	cp $< tempaarch64/main
	cd tempaarch64 && tar -cvzf ../$@ .
	rm -Rf tempaarch64

sammagents-linux-amd64-$(VERSION).tgz: build/main-linux-amd64 $(TMPL_FILES) $(STATIC_FILES)
	mkdir tempamd64
	cp -R static templates .env.example docker/* tempamd64
	cp $< tempamd64/main
	cd tempamd64 && tar -cvzf ../$@ .
	rm -Rf tempamd64

clean:
	rm -Rf temp build *.tgz
