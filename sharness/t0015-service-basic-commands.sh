#!/bin/sh

test_description="Test service installation and some basic commands"

. lib/test-lib.sh


test_expect_success "cluster-service --version succeeds" '
    ipfs-cluster-service --version >version.txt
'

test_expect_success "custer-service help output looks good" '
    ipfs-cluster-service --help | egrep -q -i "^(Usage|Commands|Description|Global Options)"
'
 
test_done
