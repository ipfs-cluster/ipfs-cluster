#!/bin/sh

set -x

test_description="Test service + ctl SSL interaction"

ssl_config="`pwd`/ssl"

. lib/test-lib.sh

test_ipfs_init
cleanup test_clean_ipfs
test_cluster_init "$ssl_config"
cleanup test_clean_cluster

test_expect_success "ssl interaction succeeds" '
    id=`cluster_id`
    test_cluster_config && ipfs-cluster-ctl --https --no-check-certificate id | egrep -q "$id"
'

test_done
