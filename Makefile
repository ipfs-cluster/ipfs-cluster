deptools=deptools
sharness = sharness/lib/sharness
GXENABLED=no # Set to yes for Gx builds.
gx=$(deptools)/gx
gx-go=$(deptools)/gx-go

# For debugging
problematic_test = TestClustersReplicationRealloc

export PATH := $(deptools):$(PATH)

all: build
clean: rwundo clean_sharness
	$(MAKE) -C cmd/ipfs-cluster-service clean
	$(MAKE) -C cmd/ipfs-cluster-ctl clean
	@rm -rf ./test/testingData
	@rm -rf ./compose

install: gx-deps
	$(MAKE) -C cmd/ipfs-cluster-service install
	$(MAKE) -C cmd/ipfs-cluster-ctl install

docker_install: docker_gx-deps
	$(MAKE) -C cmd/ipfs-cluster-service install
	$(MAKE) -C cmd/ipfs-cluster-ctl install

build: gx-deps
	go build -ldflags "-X ipfscluster.Commit=$(shell git rev-parse HEAD)"
	$(MAKE) -C cmd/ipfs-cluster-service build
	$(MAKE) -C cmd/ipfs-cluster-ctl build

service: gx-deps
	$(MAKE) -C cmd/ipfs-cluster-service ipfs-cluster-service
ctl: gx-deps
	$(MAKE) -C cmd/ipfs-cluster-ctl ipfs-cluster-ctl

gx-clean: clean
	$(MAKE) -C $(deptools) gx-clean

gx:
ifeq ($(GXENABLED),yes)
	$(MAKE) -C $(deptools) gx
endif


gx-deps: gx
ifeq ($(GXENABLED),yes)
	$(gx) install --global
	$(gx-go) rewrite
endif

# Run this target before building the docker image 
# and then gx won't attempt to pull all deps 
# from the network each time
docker_gx-deps: gx
ifeq ($(GXENABLED),yes)
	$(gx) install --local
	$(gx-go) rewrite
endif

check:
	go vet ./...
	golint -set_exit_status -min_confidence 0.3 ./...

test: gx-deps
	go test -v ./...

test_sharness: $(sharness)
	@sh sharness/run-sharness-tests.sh

test_problem: gx-deps
	go test -timeout 20m -loglevel "DEBUG" -v -run $(problematic_test)

$(sharness):
	@echo "Downloading sharness"
	@curl -L -s -o sharness/lib/sharness.tar.gz http://github.com/chriscool/sharness/archive/master.tar.gz
	@cd sharness/lib; tar -zxf sharness.tar.gz; cd ../..
	@mv sharness/lib/sharness-master sharness/lib/sharness
	@rm sharness/lib/sharness.tar.gz

clean_sharness:
	@rm -rf ./sharness/test-results
	@rm -rf ./sharness/lib/sharness
	@rm -rf sharness/trash\ directory*

rw: gx
	$(gx-go) rewrite
rwundo: gx
	$(gx-go) rewrite --undo
publish: rwundo
	$(gx) publish

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


docker-compose:
	mkdir -p compose/ipfs0 compose/ipfs1 compose/cluster0 compose/cluster1
	chmod -R 0777 compose
	CLUSTER_SECRET=$(shell od -vN 32 -An -tx1 /dev/urandom | tr -d ' \n') docker-compose up -d
	sleep 20
	docker exec cluster0 ipfs-cluster-ctl peers ls | grep -o "Sees 1 other peers" | uniq -c | grep 2
	docker exec cluster1 ipfs-cluster-ctl peers ls | grep -o "Sees 1 other peers" | uniq -c | grep 2
	docker-compose down

prcheck: gx-deps check service ctl test

.PHONY: all gx gx-deps test test_sharness clean_sharness rw rwundo publish service ctl install clean gx-clean docker
