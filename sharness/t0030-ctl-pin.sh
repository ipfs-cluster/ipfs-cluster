#!/bin/bash

test_description="Test cluster-ctl's pinning and unpinning functionality"


. lib/test-lib.sh

test_expect_success "pre-reqs enabled" '
    test_ipfs_init &&
    test_have_prereq IPFS_INIT &&
    test_cluster_init &&
    test_have_prereq CLUSTER_INIT
'

test_expect_success "pin data to cluster with ctl" '
    cid=$(echo "test" | ipfs add -q) &&
    echo $cid &&
    ipfs-cluster-ctl pin add "$cid" &&
    ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid" &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "PINNED"
'

test_expect_success "unpin data from cluster with ctl" '
    ipfs-cluster-ctl pin rm "$cid" &&
    !(ipfs-cluster-ctl pin ls "$cid" | grep -q "$cid") &&
    ipfs-cluster-ctl status "$cid" | grep -q -i "UNPINNED"   
'

cleanup test_clean_cluster
cleanup test_clean_ipfs
test_done
