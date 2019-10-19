FROM golang:1.13-stretch AS builder
MAINTAINER Hector Sanjuan <hector@protocol.ai>

# This dockerfile builds and runs ipfs-cluster-service.

ENV GOPATH     /go
ENV SRC_PATH   $GOPATH/src/github.com/ipfs/ipfs-cluster
ENV GO111MODULE on
ENV GOPROXY=https://proxy.golang.org

COPY . $SRC_PATH
WORKDIR $SRC_PATH
RUN make install

ENV SUEXEC_VERSION v0.2
ENV TINI_VERSION v0.16.1
RUN set -x \
  && cd /tmp \
  && git clone https://github.com/ncopa/su-exec.git \
  && cd su-exec \
  && git checkout -q $SUEXEC_VERSION \
  && make \
  && cd /tmp \
  && wget -q -O tini https://github.com/krallin/tini/releases/download/$TINI_VERSION/tini \
  && chmod +x tini

# Get the TLS CA certificates, they're not provided by busybox.
RUN apt-get update && apt-get install -y ca-certificates

#------------------------------------------------------
FROM busybox:1-glibc
MAINTAINER Hector Sanjuan <hector@protocol.ai>

ENV GOPATH     /go
ENV SRC_PATH   /go/src/github.com/ipfs/ipfs-cluster
ENV IPFS_CLUSTER_PATH /data/ipfs-cluster
ENV IPFS_CLUSTER_CONSENSUS crdt

EXPOSE 9094
EXPOSE 9095
EXPOSE 9096

COPY --from=builder $GOPATH/bin/ipfs-cluster-service /usr/local/bin/ipfs-cluster-service
COPY --from=builder $GOPATH/bin/ipfs-cluster-ctl /usr/local/bin/ipfs-cluster-ctl
COPY --from=builder $SRC_PATH/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --from=builder /tmp/su-exec/su-exec /sbin/su-exec
COPY --from=builder /tmp/tini /sbin/tini
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

RUN mkdir -p $IPFS_CLUSTER_PATH && \
    adduser -D -h $IPFS_CLUSTER_PATH -u 1000 -G users ipfs && \
    chown ipfs:users $IPFS_CLUSTER_PATH

VOLUME $IPFS_CLUSTER_PATH
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]

# Defaults for ipfs-cluster-service go here
CMD ["daemon"]
