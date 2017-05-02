#!/bin/sh

test_description="Test service startup and init functionality"

. lib/test-lib.sh

test_expect_success "ipfs setup" '
    test_ipfs_init &&
    test_have_prereq IPFS_INIT
'

test_expect_success "ipfs cluster setup" '
    test_cluster_init && 
    test_have_prereq CLUSTER_INIT
'

cleanup test_clean_cluster 
cleanup test_clean_ipfs
test_done
