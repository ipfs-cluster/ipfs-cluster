FROM golang:1.20-bullseye AS builder
MAINTAINER Hector Sanjuan <hector@protocol.ai>

# This dockerfile builds and runs ipfs-cluster-service.

ENV GOPATH      /go
ENV SRC_PATH    $GOPATH/src/github.com/ipfs-cluster/ipfs-cluster
ENV GO111MODULE on
ENV GOPROXY     https://proxy.golang.org

# Get the TLS CA certificates, they're not provided by busybox.
RUN apt-get update && apt-get install -y ca-certificates tini gosu

COPY --chown=1000:users go.* $SRC_PATH/
WORKDIR $SRC_PATH
RUN go mod download

COPY --chown=1000:users . $SRC_PATH
RUN git config --global --add safe.directory /go/src/github.com/ipfs-cluster/ipfs-cluster
RUN make install


#------------------------------------------------------
FROM busybox:1-glibc
MAINTAINER Hector Sanjuan <hector@protocol.ai>

ENV GOPATH                 /go
ENV SRC_PATH               /go/src/github.com/ipfs-cluster/ipfs-cluster
ENV IPFS_CLUSTER_PATH      /data/ipfs-cluster
ENV IPFS_CLUSTER_CONSENSUS crdt
ENV IPFS_CLUSTER_DATASTORE pebble

EXPOSE 9094
EXPOSE 9095
EXPOSE 9096

COPY --from=builder $GOPATH/bin/ipfs-cluster-service /usr/local/bin/ipfs-cluster-service
COPY --from=builder $GOPATH/bin/ipfs-cluster-ctl /usr/local/bin/ipfs-cluster-ctl
COPY --from=builder $GOPATH/bin/ipfs-cluster-follow /usr/local/bin/ipfs-cluster-follow
COPY --from=builder $SRC_PATH/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --from=builder /usr/bin/tini /usr/bin/tini
COPY --from=builder /usr/sbin/gosu /usr/sbin/gosu
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

RUN mkdir -p $IPFS_CLUSTER_PATH && \
    adduser -D -h $IPFS_CLUSTER_PATH -u 1000 -G users ipfs && \
    chown ipfs:users $IPFS_CLUSTER_PATH

VOLUME $IPFS_CLUSTER_PATH
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]

# Defaults for ipfs-cluster-service go here
CMD ["daemon"]
