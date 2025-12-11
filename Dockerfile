FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.25-bullseye AS builder

# This dockerfile builds and runs ipfs-cluster-service.
ARG TARGETOS TARGETARCH

ENV GOPATH=/go
ENV SRC_PATH=$GOPATH/src/github.com/ipfs-cluster/ipfs-cluster
ENV GOPROXY=https://proxy.golang.org

COPY --chown=1000:users go.* $SRC_PATH/
WORKDIR $SRC_PATH
RUN go mod download -x

COPY --chown=1000:users . $SRC_PATH
RUN git config --global --add safe.directory /go/src/github.com/ipfs-cluster/ipfs-cluster

ENV CGO_ENABLED=0
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH make build

#------------------------------------------------------
FROM alpine:3.21

LABEL org.opencontainers.image.source=https://github.com/ipfs-cluster/ipfs-cluster
LABEL org.opencontainers.image.description="Pinset orchestration for IPFS"
LABEL org.opencontainers.image.licenses=MIT+APACHE_2.0

# Install binaries for $TARGETARCH
RUN apk add --no-cache tini su-exec ca-certificates

ENV GOPATH=/go
ENV SRC_PATH=/go/src/github.com/ipfs-cluster/ipfs-cluster
ENV IPFS_CLUSTER_PATH=/data/ipfs-cluster
ENV IPFS_CLUSTER_CONSENSUS=crdt

EXPOSE 9094
EXPOSE 9095
EXPOSE 9096

COPY --from=builder $SRC_PATH/cmd/ipfs-cluster-service/ipfs-cluster-service /usr/local/bin/ipfs-cluster-service
COPY --from=builder $SRC_PATH/cmd/ipfs-cluster-ctl/ipfs-cluster-ctl /usr/local/bin/ipfs-cluster-ctl
COPY --from=builder $SRC_PATH/cmd/ipfs-cluster-follow/ipfs-cluster-follow /usr/local/bin/ipfs-cluster-follow
COPY --from=builder $SRC_PATH/docker/entrypoint.sh /usr/local/bin/entrypoint.sh

RUN mkdir -p $IPFS_CLUSTER_PATH && \
    adduser -D -h $IPFS_CLUSTER_PATH -u 1000 -G users ipfs && \
    chown ipfs:users $IPFS_CLUSTER_PATH

VOLUME $IPFS_CLUSTER_PATH
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]

# Defaults for ipfs-cluster-service go here
CMD ["daemon"]
