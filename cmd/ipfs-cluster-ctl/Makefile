# go source files
SRC := $(shell find .. -type f -name '*.go')

all: ipfs-cluster-ctl

ipfs-cluster-ctl: $(SRC)
	go build -mod=readonly

build: ipfs-cluster-ctl

install:
	go install

clean:
	rm -f ipfs-cluster-ctl

.PHONY: clean install build
