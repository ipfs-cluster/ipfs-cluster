#!/bin/sh

test_description="Test ctl's status reporting functionality.  Test errors on incomplete commands"

. lib/test-lib.sh

test_expect_success "cluster-ctl version looks good" '
    ipfs-cluster-ctl version >version.txt &&
    egrep "[0-9]+\.[0-9]+\.[0-9]" version.txt >/dev/null &&
    rm version.txt
'

test_expect_success "cluster-ctl can read id" '
    ipfs-cluster-ctl id >id.txt &&
    grep "> Addresses:" id.txt >/dev/null &&
    grep "> IPFS: " id.txt >/dev/null &&
    rm id.txt
'

test_expect_success "cluster-ctl list 0 peers" '
    ipfs-cluster-ctl peers ls >peers.txt &&
    grep "| 0 peers" peers.txt >/dev/null &&
    rm peers.txt
'

test_expect_failure "cluster-ctl add need multiaddress" '
    ipfs-cluster-ctl peers add 
'

test_expect_failure "cluster-ctl rm need multihash" '
    ipfs-cluster-ctl peers rm 
'

test_expect_success "empty cluster-ctl status succeeds" '
    ipfs-cluster-ctl status 
'

test_expect_success "empty cluster-ctl sync succeeds" '
    ipfs-cluster-ctl sync 
'

test_expect_failure "empty cluster_ctl recover needs CID" '
    ipfs-cluster-ctl recover 
'

test_expect_success "pin ls " '
    ipfs-cluster-ctl pin ls 
'

test_done
