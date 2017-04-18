#!/bin/sh
#
# MIT Licensed
#

test_description="Cleanup state from tests (kill ipfs daemon)"

. lib/test-lib.sh

#test_expect_success "Kill ipfs daemon" '
#    ps | grep "ipfs daemon" | grep -v "grep" > daemon_ps.txt &&
#    sed -i '''' ''s/^ *//'' daemon_ps.txt &&
#    awk ''{print$1}'' daemon_ps.txt > daemon_pid.txt &&
#    kill -9 $(< daemon_pid.txt) 
#'

test_expect_success "Kill cluster-service " '
    kill -0 $(< ../.test_config/sPID.txt)
'

test_expect_success "Kill ipfs dameon " '
    kill -0 $(< ../.test_ipfs/dPID.txt)
'

test_done
