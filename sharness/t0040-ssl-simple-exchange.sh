#!/bin/bash

test_description="Test service + ctl SSL interaction"

ssl_config="`pwd`/config/ssl"

. lib/test-lib.sh

test_ipfs_init

test_cluster_init "$ssl_config"
cleanup test_clean_cluster

test_expect_success "prerequisites" '
    test_have_prereq IPFS && test_have_prereq CLUSTER
'

test_expect_success "ssl interaction succeeds" '
    id=`cluster_id`
    ipfs-cluster-ctl --https --no-check-certificate id | egrep -q "$id"
'

test_clean_ipfs

test_done
