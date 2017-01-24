all: ipfs-cluster-service

ipfs-cluster-service:
	go build

build: ipfs-cluster-service

install:
	go install -ldflags "-X ipfscluster.commit=$(shell git rev-parse HEAD)"

clean:
	rm -f ipfs-cluster-service

.PHONY: clean install build
