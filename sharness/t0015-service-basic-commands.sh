#!/bin/sh

test_description="Test service installation and some basic commands"

. lib/test-lib.sh


test_expect_success "cluster-service --version succeeds" '
    ipfs-cluster-service --version >version.txt
'

test_expect_success "cluster-service --version output looks good" '
    egrep "^ipfs-cluster-service version [0-9]+\.[0-9]+\.[0-9]" version.txt >/dev/null &&
    rm version.txt
'

test_expect_success "cluster-service --help and -h succeed" '
    ipfs-cluster-service --help &&
    ipfs-cluster-service -h 
'

test_expect_success "cluster-service help and h succeed" '
    ipfs-cluster-service help &&
    ipfs-cluster-service h 
'

test_expect_success "cluster-service help options match" '
    ipfs-cluster-service help >help.txt &&
    ipfs-cluster-service h >help1.txt &&
    ipfs-cluster-service --help >help2.txt &&
    ipfs-cluster-service --h >help3.txt && 
    diff help.txt help1.txt &&
    diff help.txt help2.txt &&
    diff help.txt help3.txt &&
    rm help1.txt help2.txt help3.txt
'

test_expect_success "custer-service help output looks good" '
    egrep -i "^Usage" help.txt >/dev/null &&
    egrep -i "^Commands" help.txt >/dev/null &&
    egrep -i "^Description" help.txt >/dev/null &&
    egrep -i "^Global Options" help.txt >/dev/null &&   
    rm help.txt
'
 
test_done
