FROM ipfs/go-ipfs:latest
MAINTAINER Hector Sanjuan <hector@protocol.ai>

EXPOSE 9094
EXPOSE 9095
EXPOSE 9096

ENV GOPATH     /go
ENV PATH       /go/bin:$PATH
ENV SRC_PATH   /go/src/github.com/ipfs/ipfs-cluster
ENV IPFS_CLUSTER_PATH /data/ipfs-cluster

USER root

VOLUME $IPFS_CLUSTER_PATH

COPY . $SRC_PATH

RUN apk add --no-cache --virtual cluster-deps make musl-dev go git \
    && apk add --no-cache jq \
    && go get -u github.com/whyrusleeping/gx \
    && go get -u github.com/whyrusleeping/gx-go

RUN cd $SRC_PATH && go get github.com/hashicorp/raft \
    && go get github.com/hashicorp/raft-boltdb \ 
    && go get github.com/hsanjuan/go-libp2p-gorpc \
    && go get github.com/ipfs/go-cid \
    && go get github.com/libp2p/go-libp2p-consensus \
    && go get github.com/libp2p/go-libp2p-raft \ 
    && go get github.com/libp2p/go-libp2p-swarm \
    && go get github.com/libp2p/go-libp2p/p2p/host/basic \
    && go get github.com/gorilla/mux \
    && go get github.com/urfave/cli

RUN cd $SRC_PATH && gx install --global

RUN cd $SRC_PATH && gx-go --verbose rewrite 

RUN cd $SRC_PATH && go build

RUN cd $SRC_PATH \
    && make -C ipfs-cluster-service install \
    && make -C ipfs-cluster-ctl install \
    && cp docker/entrypoint.sh /usr/local/bin/start-daemons.sh \
    && chmod +x /usr/local/bin/start-daemons.sh \
    && apk del --purge cluster-deps \
    && cd / && rm -rf /go/src /go/bin/gx /go/bin/gx-go


ENTRYPOINT ["/usr/local/bin/start-daemons.sh"]

CMD ["$IPFS_CLUSTER_OPTS"]
