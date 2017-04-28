#!/bin/sh

test_description="Test service startup and init functionality"

. lib/test-lib.sh

test_expect_success "ipfs is installed on this machine" '
   ipfs help >ipfs_help.txt &&
   egrep -i "^Usage" ipfs_help.txt >/dev/null  
'

test_expect_success "launch ipfs daemon" '
   mkdir -p ../.test_ipfs &&
   IPFS_PATH="../.test_ipfs" eval '"'"'ipfs init'"'"' &&
   IPFS_PATH="../.test_ipfs" eval '"'"'ipfs daemon & echo $! >../dPID.txt'"'"' &&
   sleep 2 
'

test_expect_success "test config folder exists" '
    mkdir -p ../.test_config
'

test_expect_success "init cluster-service" '
    ipfs-cluster-service -f --config ../.test_config init 2>service_init.txt &&
    grep "configuration written" service_init.txt >/dev/null &&
    rm service_init.txt
'

test_expect_success "run cluster-service" '
    ipfs-cluster-service --config ../.test_config 2>service_start.txt &
    echo $!>../sPID.txt &&
    sleep 2  &&
    egrep -i "ready" service_start.txt >/dev/null &&
    rm service_start.txt
'

test_done
