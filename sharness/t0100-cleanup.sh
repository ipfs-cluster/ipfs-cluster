#!/bin/sh

test_description="Cleanup state from tests (kill ipfs daemon)"

. lib/test-lib.sh

test_expect_success "Kill cluster-service " '
    kill -1 $(< ../sPID.txt) &&
    rm ../sPID.txt
'

test_expect_success "Kill ipfs dameon " '
    kill -1 $(< ../dPID.txt) &&
    rm ../dPID.txt
'

test_expect_success "Remove config directories" '
    rm -rf ../.test_ipfs &&
    rm -rf ../.test_config
'


test_done
