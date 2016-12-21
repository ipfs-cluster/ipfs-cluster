all: install
clean:
	$(MAKE) -C ipfscluster-server clean
install: deps
	$(MAKE) -C ipfscluster-server install
server: deps
	$(MAKE) -C ipfscluster-server ipfscluster-server
gx:
	go get github.com/whyrusleeping/gx
	go get github.com/whyrusleeping/gx-go
deps: gx
	go get github.com/gorilla/mux
	go get github.com/hashicorp/raft
	go get github.com/hashicorp/raft-boltdb
	go get github.com/ugorji/go/codec

	gx --verbose install --global
	gx-go rewrite
test: deps
	go test -tags silent -v -covermode count -coverprofile=coverage.out .
rw:
	gx-go rewrite
rwundo:
	gx-go rewrite --undo
publish: rwundo
	gx publish
.PHONY: all gx deps test rw rwundo publish server install clean
