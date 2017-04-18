#!/bin/sh
#
# MIT Licensed
#

test_description = "Test service installation and some basic commands"

. lib/test-lib.sh


test_expect_success "cluster-service --version succeeds" '
    ipfs-cluster-service --version >version.txt
'

test_expect_success "cluster-service --version output looks good" '

'

test_expect_success "cluster-service --help and -h succeed" '

'

test_expect_success "cluster-service help and h succeed" '

'

test_expect_success "cluster-service help options match" '

'

test_expect_success "custer-service help output looks good" '

'

test_done
