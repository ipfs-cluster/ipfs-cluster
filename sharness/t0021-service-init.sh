#!/bin/bash

test_description="Test init functionality"

. lib/test-lib.sh

test_expect_success "cluster-service init with --peers succeeds and fills peerstore" '
    PEER1=/ip4/192.168.0.129/tcp/9196/p2p/12D3KooWRN8KRjpyg9rsW2w7StbBRGper65psTZm68cjud9KAkaW
    PEER2=/ip4/192.168.0.129/tcp/9196/p2p/12D3KooWPwrYNj7VficHw5qYidepMGA85756kYgMdNmRM9A1ZHjN
    ipfs-cluster-service --config "test-config" init --peers $PEER1,$PEER2 &&
    grep -q $PEER1 test-config/peerstore &&
    grep -q $PEER2 test-config/peerstore
'

test_expect_success "cluster-service init without --peers succeeds and creates empty peerstore" '
    ipfs-cluster-service --config "test-config" init -f &&
    [ -f "test-config/peerstore" ] &&
    [ ! -s "test-config/peerstore" ]
'

test_clean_cluster

test_done
