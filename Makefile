gx_version=v0.10.0
gx-go_version=v1.4.0
gx=gx_$(gx_version)
gx-go=gx-go_$(gx-go_version)
gx_bin=deptools/$(gx)
gx-go_bin=deptools/$(gx-go)
bin_env=$(shell go env GOHOSTOS)-$(shell go env GOHOSTARCH)

all: service ctl
clean: rwundo
	$(MAKE) -C ipfs-cluster-service clean
	$(MAKE) -C ipfs-cluster-ctl clean
	rm -rf deptools
install: deps
	$(MAKE) -C ipfs-cluster-service install
	$(MAKE) -C ipfs-cluster-ctl install

build: deps
	go build -ldflags "-X ipfscluster.Commit=$(shell git rev-parse HEAD)"
	$(MAKE) -C ipfs-cluster-service build
	$(MAKE) -C ipfs-cluster-ctl build

service: deps
	$(MAKE) -C ipfs-cluster-service ipfs-cluster-service
ctl: deps
	$(MAKE) -C ipfs-cluster-ctl ipfs-cluster-ctl

$(gx_bin):
	@echo "Downloading gx"
	@mkdir -p ./deptools
	@wget -nc -q -O $(gx_bin).tgz https://dist.ipfs.io/gx/$(gx_version)/$(gx)_$(bin_env).tar.gz
	@tar -zxf $(gx_bin).tgz -C deptools --strip-components=1 gx/gx
	@mv deptools/gx $(gx_bin)
	@rm $(gx_bin).tgz

$(gx-go_bin):
	echo "Downloading gx-go"
	mkdir -p ./deptools
	wget -nc -q -O $(gx-go_bin).tgz https://dist.ipfs.io/gx-go/$(gx-go_version)/$(gx-go)_$(bin_env).tar.gz
	tar -zxf $(gx-go_bin).tgz -C deptools --strip-components=1 gx-go/gx-go
	mv deptools/gx-go $(gx-go_bin)
	rm $(gx-go_bin).tgz

gx: $(gx_bin) $(gx-go_bin)

deps: gx
	go get github.com/gorilla/mux
	go get github.com/hashicorp/raft
	go get github.com/hashicorp/raft-boltdb
	go get github.com/ugorji/go/codec
	$(gx_bin) --verbose install --global
	$(gx-go_bin) rewrite
test: deps
	go test -tags silent -v -covermode count -coverprofile=coverage.out .
rw:
	$(gx-go_bin) rewrite
rwundo:
	$(gx-go_bin) rewrite --undo
publish: rwundo
	$(gx_bin) publish
.PHONY: all gx deps test rw rwundo publish service ctl install clean
