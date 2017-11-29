#!/bin/bash

test_description="Test failure when server not using SSL but client requests it"

. lib/test-lib.sh

test_ipfs_init
cleanup test_clean_ipfs
test_cluster_init
cleanup test_clean_cluster

test_expect_success "prerequisites" '
    test_have_prereq IPFS && test_have_prereq CLUSTER
'

test_expect_success "ssl enforced by client" '
    id=`cluster_id`
    test_must_fail ipfs-cluster-ctl --https --no-check-certificate id
'

test_done
