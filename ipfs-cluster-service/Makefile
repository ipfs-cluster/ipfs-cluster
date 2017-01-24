all: ipfs-cluster-service

ipfs-cluster-service:
	go build -ldflags "-X main.commit=$(shell git rev-parse HEAD)"

build: ipfs-cluster-service

install:
	go install -ldflags "-X ipfscluster.Commit=$(shell git rev-parse HEAD)"

clean:
	rm -f ipfs-cluster-service

.PHONY: clean install build
