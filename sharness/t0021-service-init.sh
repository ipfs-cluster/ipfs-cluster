#!/bin/bash

test_description="Test init functionality"

. lib/test-lib.sh
test_ipfs_init

test_expect_success "prerequisites" '
    test_have_prereq IPFS
'

PEERS=/ip4/192.168.0.129/tcp/9196/ipfs/12D3KooWRN8KRjpyg9rsW2w7StbBRGper65psTZm68cjud9KAkaW,/ip4/192.168.0.129/tcp/9196/ipfs/12D3KooWPwrYNj7VficHw5qYidepMGA85756kYgMdNmRM9A1ZHjN

test_expect_success "cluster-service init with --peers succeeds and fills peerstore" '
    ipfs-cluster-service --config "test-config" init --peers $PEERS
    [ -f "test-config/peerstore" ]
    IFS=','
    read -ra P <<< $PEERS
    grep -q ${P[0]} "test-config/peerstore"
    grep -q ${P[1]} "test-config/peerstore"
'

test_expect_success "cluster-service init without --peers succeeds and creates empty peerstore" '
    ipfs-cluster-service -f --config "test-config" init
    [ -f "test-config/peerstore" ]
    [ -z `cat "test-config/peerstore"` ]
'

test_clean_ipfs
test_clean_cluster

test_done
