#!/bin/sh
#
# MIT LICENSED
#
test_description="Test service startup and init functionality"

. lib/test-lib.sh

test_expect_success "ipfs is installed on this machine" '
   ipfs help >ipfs_help.txt &&
   egrep -i "^Usage" ipfs_help.txt >/dev/null  
'

# For now launching ipfs daemon externally, later will 
# get this working automatically through tests

test_expect_success "launch ipfs daemon" '
    mkdir -p ../.test_ipfs &&
    [ -d ../.test_ipfs ] &&
    IPFS_PATH="../.test_ipfs" eval ''ipfs init'' &&
    IPFS_PATH="../.test_ipfs" eval ''ipfs daemon & echo $! >../dPID.txt'' &&
    sleep 3
'

test_expect_success "test config folder exists" '
    mkdir -p ../.test_config &&
    [ -d ../.test_config ]
'

test_expect_success "init cluster-service" '
    ipfs-cluster-service -f --config ../.test_config init 2>service_init.txt &&
    grep "configuration written" service_init.txt >/dev/null &&
    rm service_init.txt
'

test_expect_success "run cluster-service" '
    ipfs-cluster-service --config ../.test_config 2>service_start.txt &
    echo $!>../sPID.txt &&
    sleep 3  &&
    egrep -i "ready" service_start.txt >/dev/null &&
    rm service_start.txt
'

#test_expect_success "ipfs daemon launch and tear down" '
#    ipfs daemon >daemon_out.txt 2>daemon_err &
#    IPFS_PID=$!
#    sleep 2 &&
#    if ! kill -0 $IPFS_PID; then cat daemon_err; return 1; fi
#'

#test_expect_success "ipfs daemon launced successfully" '
#    return 1 &&
#    ipfs daemon >daemon_out.txt 2>daemon_err &
#    sleep 5 &&
#    egrep -i "Initializing daemon" daemon_out.txt >/dev/null &&
#    egrep -i "API server listening" daemon_out.txt >/dev/null &&
#    egrep -i "Daemon is ready" daemon_out.txt > /dev/null
#'

test_done
