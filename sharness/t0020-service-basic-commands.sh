#!/bin/bash

test_description="Test service startup and init functionality"

. lib/test-lib.sh
test_ipfs_init
cleanup test_clean_ipfs
test_cluster_init
cleanup test_clean_cluster

test_expect_success "prerequisites" '
    test_have_prereq IPFS &&
    test_have_prereq CLUSTER
'

test_expect_success JQ "ipfs cluster config valid" '
    test_cluster_config
'

test_expect_success "custer-service help output looks good" '
    ipfs-cluster-service --help | egrep -q -i "^(Usage|Commands|Description|Global Options)"
'

test_expect_success "cluster-service --version succeeds and matches ctl" '
    export SERV_VERSION=`ipfs-cluster-service --version | grep -Po "\d+\.\d+\.\d+"`
    export CTL_VERSION=`ipfs-cluster-ctl --version | grep -Po "\d+\.\d+\.\d+"`
    [ "$SERV_VERSION" = "$CTL_VERSION" ]
'

test_expect_success "starting a second cluster-service process fails" '
    test_expect_code 1 ipfs-cluster-service --config "test-config"    
'

test_done
