#!/bin/bash

test_description="Test cluster-ctl's pinning and unpinning functionality"


. lib/test-lib.sh


test_expect_success "pin data to cluster with ctl" '
    IPFS_PATH="../.test_ipfs" eval '"'"'ipfs add ../lib/sharness/sharness.sh >add_output.txt'"'"' &&
    grep "added" add_output.txt &&
    awk '"'"'{print $2}'"'"' add_output.txt > CID.txt &&
    ipfs-cluster-ctl pin add $(< CID.txt) >pin_output.txt 
    grep " PINNED" pin_output.txt >/dev/null &&
    rm add_output.txt pin_output.txt
'

test_expect_success "unpin data from cluster with ctl" '
   IPFS_PATH="../.test_ipfs" eval '"'"'ipfs-cluster-ctl pin rm $(< CID.txt) >unpin_output.txt'"'"' &&
   grep "UNPINNED" unpin_output.txt >/dev/null &&
   rm unpin_output.txt CID.txt
'


test_done
