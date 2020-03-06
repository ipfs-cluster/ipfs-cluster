# go source files
SRC := $(shell find ../.. -type f -name '*.go')
GOPATH := $(shell go env GOPATH)
GOFLAGS := "-asmflags=all='-trimpath=$(GOPATH)'" "-gcflags=all='-trimpath=$(GOPATH)'"

all: ipfs-cluster-follow

ipfs-cluster-follow: $(SRC)
	go build $(GOFLAGS) -mod=readonly -ldflags "-X main.commit=$(shell git rev-parse HEAD)"

build: ipfs-cluster-follow

install:
	go install $(GOFLAGS) -ldflags "-X main.commit=$(shell git rev-parse HEAD)"

clean:
	rm -f ipfs-cluster-follow

.PHONY: clean install build
