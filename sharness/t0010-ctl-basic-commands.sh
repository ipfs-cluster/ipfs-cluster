#!/bin/bash

test_description="Test ctl installation and some basic commands"

. lib/test-lib.sh


test_expect_success "current dir is writeable" '
    echo "Writability check" >test.txt &&
    test_when_finished "rm test.txt"
'

test_expect_success "cluster-ctl --version succeeds" '
    ipfs-cluster-ctl --version
'

test_expect_success "cluster-ctl help commands succeed" '
    ipfs-cluster-ctl --help &&
    ipfs-cluster-ctl -h &&
    ipfs-cluster-ctl h &&
    ipfs-cluster-ctl help
'

test_expect_success "cluster-ctl help has 120 char limits" '
    ipfs-cluster-ctl --help >help.txt &&
    test_when_finished "rm help.txt" &&
    LENGTH="$(cat help.txt | awk '"'"'{print length }'"'"' | sort -nr | head -n 1)" &&
    [ ! "$LENGTH" -gt 120 ]
'

test_expect_success "cluster-ctl help output looks good" '
    ipfs-cluster-ctl --help | egrep -q -i "^(Usage|Commands|Global options)"
'

test_expect_success "cluster-ctl commands output looks good" '
    ipfs-cluster-ctl commands > commands.txt &&
    test_when_finished "rm commands.txt" &&
    egrep -q "ipfs-cluster-ctl id" commands.txt &&
    egrep -q "ipfs-cluster-ctl peers" commands.txt &&
    egrep -q "ipfs-cluster-ctl pin" commands.txt &&
    egrep -q "ipfs-cluster-ctl status" commands.txt &&
    egrep -q "ipfs-cluster-ctl recover" commands.txt &&
    egrep -q "ipfs-cluster-ctl version" commands.txt &&
    egrep -q "ipfs-cluster-ctl commands" commands.txt
'

test_expect_success "All cluster-ctl command docs are 120 columns or less" '
    export failure="0" &&
    ipfs-cluster-ctl commands | awk "NF" >commands.txt &&
    test_when_finished "rm commands.txt" &&
    while read cmd
    do
        LENGTH="$($cmd --help | awk "{ print length }" | sort -nr | head -n 1)"
        [ "$LENGTH" -gt 120 ] &&
            { echo "$cmd" help text is longer than 119 chars "($LENGTH)"; export failure="1"; }
    done <commands.txt

    if [ $failure -eq "1" ]; then
        return 1
    fi
'
test_done
