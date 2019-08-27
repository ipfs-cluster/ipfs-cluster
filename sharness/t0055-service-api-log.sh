#!/bin/bash

test_description="Test HTTP API log functionality"

config="`pwd`/config/test"

. lib/test-lib.sh

test_ipfs_init
test_cluster_init "$config"

test_expect_success IPFS,CLUSTER "log file which gets populated on API hits" '
    [ -f "test-config/http.log" ] &&
    [ ! -s "test-config/http.log" ] &&
    ipfs-cluster-ctl id &&
    [ -s "test-config/http.log" ]
'

test_clean_ipfs
test_clean_cluster

test_done
