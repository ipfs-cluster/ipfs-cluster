# go source files
SRC := $(shell find ../.. -type f -name '*.go')
GOPATH := $(shell go env GOPATH)
GOFLAGS := "-asmflags=all='-trimpath=$(GOPATH)'" "-gcflags=all='-trimpath=$(GOPATH)'"

all: ipfs-cluster-ctl

ipfs-cluster-ctl: $(SRC)
	go build $(GOFLAGS) -mod=readonly

build: ipfs-cluster-ctl

install:
	go install $(GOFLAGS)

clean:
	rm -f ipfs-cluster-ctl

.PHONY: clean install build
