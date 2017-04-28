#!/bin/sh
#
# MIT Licensed
#

test_description="Test ctl installation and some basic commands"

. lib/test-lib.sh

test_expect_success "current dir is writeable" '
    echo "Writability check" >test.txt
'

test_expect_success "cluster-ctl --version succeeds" '
    ipfs-cluster-ctl --version >version.txt
'

test_expect_success "cluster-ctl --version output looks good" '
   egrep "^ipfs-cluster-ctl version [0-9]+\.[0-9]+\.[0-9]" version.txt >/dev/null &&
   rm version.txt
'

test_expect_success "cluster-ctl --help and -h succeed" '
    ipfs-cluster-ctl --help &&
    ipfs-cluster-ctl -h 
'

test_expect_success "cluster-ctl help and h succeed" '
    ipfs-cluster-ctl h &&
    ipfs-cluster-ctl help
'

test_expect_success "All help options match" '
    ipfs-cluster-ctl help >help.txt &&
    ipfs-cluster-ctl h >help1.txt &&
    ipfs-cluster-ctl --help >help2.txt &&
    ipfs-cluster-ctl --h >help3.txt && 
    diff help.txt help1.txt &&
    diff help.txt help2.txt &&
    diff help.txt help3.txt &&
    rm help1.txt help2.txt help3.txt
'

test_expect_success "cluster-ctl help output looks good" '
    egrep -i "^Usage" help.txt >/dev/null &&
    egrep -i "^Commands" help.txt >/dev/null &&
    egrep -i "^Global Options" help.txt >/dev/null &&
    rm help.txt
'

test_expect_success "cluster-ctl commands succeeds" '
    ipfs-cluster-ctl commands >unfmt_commands.txt &&
    awk ''NF'' unfmt_commands.txt >commands.txt
'

test_expect_success "cluster-ctl commands output looks good" '
    grep "ipfs-cluster-ctl id" commands.txt &&
    grep "ipfs-cluster-ctl peers" commands.txt &&
    grep "ipfs-cluster-ctl pin" commands.txt &&
    grep "ipfs-cluster-ctl status" commands.txt &&
    grep "ipfs-cluster-ctl sync" commands.txt &&
    grep "ipfs-cluster-ctl recover" commands.txt &&
    grep "ipfs-cluster-ctl version" commands.txt &&
    grep "ipfs-cluster-ctl commands" commands.txt 
'

test_expect_success "All commands accept --help" '
    echo 0 > fail
    while read -r cmd
    do 
        $cmd --help </dev/null >/dev/null ||
            { echo $cmd does not accept --help; echo 1 > fail; }
    done <commands.txt

    if [$(cat fail) = 1 ]; then
        return 1
    fi
'

test_expect_success "All cluster-ctl command docs are 80 columns or less" '
   echo 0 > failure 
   while read cmd
   do
       LENGTH="$($cmd --help | awk "{ print length }" | sort -nr | head -1)"
       [ $LENGTH -gt 80 ] &&
           { echo "$cmd" help text is longer than 79 chars "($LENGTH)"; echo 1 > failure; }
   done <commands.txt

   if [ $(cat failure) = 1 ]; then
       return 1
   fi
   rm commands.txt
'
test_done

