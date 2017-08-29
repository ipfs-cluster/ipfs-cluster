#!/bin/sh

test_description="Test service + ctl SSL interaction"

config="`pwd`/config/ssl-basic_auth"

. lib/test-lib.sh

test_ipfs_init
cleanup test_clean_ipfs
test_cluster_init "$config"
cleanup test_clean_cluster

test_expect_success "prerequisites" '
    test_have_prereq IPFS && test_have_prereq CLUSTER
'

test_expect_success "ssl interaction succeeds" '
    id=`cluster_id`
    ipfs-cluster-ctl --no-check-certificate --basic-auth "testuser:testpass" id | egrep -q "$id"
'

test_done
