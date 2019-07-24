#!/bin/bash

test_description="Test init functionality"

. lib/test-lib.sh

test_expect_success "cluster-service init with --peers succeeds and fills peerstore" '
    PEER1=/ip4/192.168.0.129/tcp/9196/ipfs/12D3KooWRN8KRjpyg9rsW2w7StbBRGper65psTZm68cjud9KAkaW
    PEER2=/ip4/192.168.0.129/tcp/9196/ipfs/12D3KooWPwrYNj7VficHw5qYidepMGA85756kYgMdNmRM9A1ZHjN
    echo $PEER1 >> testPeerstore
    echo $PEER2 >> testPeerstore
    ipfs-cluster-service --config "test-config" init --peers $PEER1,$PEER2 &&
    [ -f "test-config/peerstore" ] &&
    test_cmp testPeerstore test-config/peerstore &&
    rm testPeerstore
'

test_expect_success "cluster-service init without --peers succeeds and creates empty peerstore" '
    ipfs-cluster-service -f --config "test-config" init &&
    [ -f "test-config/peerstore" ] &&
    [ -z `cat "test-config/peerstore"` ]
'

test_clean_cluster

test_done
