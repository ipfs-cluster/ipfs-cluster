# go source files
SRC := $(shell find .. -type f -name '*.go')

all: ipfs-cluster-follow

ipfs-cluster-follow: $(SRC)
	go build -mod=readonly -ldflags "-X main.commit=$(shell git rev-parse HEAD)"

build: ipfs-cluster-follow

install:
	go install -ldflags "-X main.commit=$(shell git rev-parse HEAD)"

clean:
	rm -f ipfs-cluster-follow

.PHONY: clean install build
