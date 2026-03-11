
SRC_FILES=$(wildcard ./pkg/*/*.go)
TMPL_FILES=$(wildcard ./templates/*)
STATIC_FILES=$(wildcard ./static/*)

all: build/main

build:
	mkdir -p build

build/main: ./cmd/dashboard/main.go build $(SRC_FILES)
	go build -o $@ $<

dist: sammagents.tgz

sammagents.tgz: build/main $(TMPL_FILES) $(STATIC_FILES)
	mkdir temp
	cp -R static templates .env.example build/* queues.json docker/* temp
	cd temp && tar -cvzf ../sammagents.tgz .
	rm -Rf temp

clean:
	rm -Rf temp build *.tgz