NAME := ipfs-cluster-test
REGISTRY := docker.io/controlplane
GIT_TAG ?= $(shell bash -c 'TAG=$$(git tag | tail -n1); echo "$${TAG:-none}"')

CONTAINER_TAG ?= $(GIT_TAG)
CONTAINER_NAME := $(REGISTRY)/$(NAME):$(CONTAINER_TAG)
CONTAINER_NAME_LATEST := $(REGISTRY)/$(NAME):latest

gx_version=v0.13.0
gx-go_version=v1.7.0

deptools=deptools

gx=gx_$(gx_version)
gx-go=gx-go_$(gx-go_version)
gx_bin=$(deptools)/$(gx)
gx-go_bin=$(deptools)/$(gx-go)
bin_env=$(shell go env GOHOSTOS)-$(shell go env GOHOSTARCH)
sharness = sharness/lib/sharness

# For debugging
problematic_test = TestClustersReplicationRealloc

export PATH := $(deptools):$(PATH)

all: service ctl
clean: rwundo clean_sharness
	$(MAKE) -C ipfs-cluster-service clean
	$(MAKE) -C ipfs-cluster-ctl clean

gx-clean: clean
	@rm -f $(deptools)/*

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
	mkdir -p ./$(deptools)
	rm -f $(deptools)/gx
	wget -nc -O $(gx_bin).tgz https://dist.ipfs.io/gx/$(gx_version)/$(gx)_$(bin_env).tar.gz
	tar -zxf $(gx_bin).tgz -C $(deptools) --strip-components=1 gx/gx
	mv $(deptools)/gx $(gx_bin)
	ln -s $(gx) $(deptools)/gx
	rm $(gx_bin).tgz

$(gx-go_bin):
	@echo "Downloading gx-go"
	mkdir -p ./$(deptools)
	rm -f $(deptools)/gx-go
	wget -nc -O $(gx-go_bin).tgz https://dist.ipfs.io/gx-go/$(gx-go_version)/$(gx-go)_$(bin_env).tar.gz
	tar -zxf $(gx-go_bin).tgz -C $(deptools) --strip-components=1 gx-go/gx-go
	mv $(deptools)/gx-go $(gx-go_bin)
	ln -s $(gx-go) $(deptools)/gx-go
	rm $(gx-go_bin).tgz

gx: $(gx_bin) $(gx-go_bin)

deps: gx
	$(gx_bin) install --global
	$(gx-go_bin) rewrite

check:
	go vet ./...
	golint -set_exit_status -min_confidence 0.3 ./...

test: deps
	go test -loglevel "CRITICAL" -v ./...

test_sharness: $(sharness)
	@sh sharness/run-sharness-tests.sh

test_problem: deps
	go test -timeout 20m -loglevel "DEBUG" -v -run $(problematic_test)

test_acceptance: ## run acceptance tests
	@echo "+ $@"
	./test/test-acceptance.sh

$(sharness):
	@echo "Downloading sharness"
	@wget -q -O sharness/lib/sharness.tar.gz http://github.com/chriscool/sharness/archive/master.tar.gz
	@cd sharness/lib; tar -zxf sharness.tar.gz; cd ../..
	@mv sharness/lib/sharness-master sharness/lib/sharness
	@rm sharness/lib/sharness.tar.gz

clean_sharness:
	@rm -rf ./sharness/test-results
	@rm -rf ./sharness/lib/sharness
	@rm -rf sharness/trash\ directory*

rw: gx
	$(gx-go_bin) rewrite
rwundo: gx
	$(gx-go_bin) rewrite --undo
publish: rwundo
	$(gx_bin) publish

docker:
	docker build -t cluster-image -f Dockerfile .
	docker run --name tmp-make-cluster -d --rm cluster-image && sleep 4
	docker exec tmp-make-cluster sh -c "ipfs-cluster-ctl version"
	docker exec tmp-make-cluster sh -c "ipfs-cluster-service -v"
	docker kill tmp-make-cluster
	docker build -t cluster-image-test -f Dockerfile-test .
	docker run --name tmp-make-cluster-test -d --rm cluster-image && sleep 8
	docker exec tmp-make-cluster-test sh -c "ipfs-cluster-ctl version"
	docker exec tmp-make-cluster-test sh -c "ipfs-cluster-service -v"
	docker kill tmp-make-cluster-test

docker-build-test-image: ## builds the test docker image
	@echo "+ $@"
	docker build -f Dockerfile-test --tag "$(CONTAINER_NAME)" .
	docker tag "$(CONTAINER_NAME)" "$(CONTAINER_NAME_LATEST)"
	@echo "Successfully tagged $(CONTAINER_NAME) as $(CONTAINER_NAME_LATEST)"

docker-push-test-image: ## pushes the test docker image
	@echo "+ $@"
	docker push "$(CONTAINER_NAME)"
	docker push "$(CONTAINER_NAME_LATEST)"

help: ## parse jobs and descriptions from this Makefile
	@grep -E '^[ a-zA-Z0-9_-]+:([^=]|$$)' $(MAKEFILE_LIST) \
		| grep -Ev '^help\b[[:space:]]*:' \
		| sort \
		| awk 'BEGIN {FS = ":.*?##"}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: all gx deps test test_sharness clean_sharness rw rwundo publish service ctl install clean gx-clean docker docker-build-test docker-push-test help
