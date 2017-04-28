#!/bin/sh

test_description="Test ctl installation and some basic commands"

. lib/test-lib.sh

test_expect_success "current dir is writeable" '
    echo "Writability check" >test.txt &&
    test_when_finished "rm test.txt"
'

# TODO Make a comparison between cluster-ctl and cluster-service version
test_expect_success "cluster-ctl --version succeeds" '
    ipfs-cluster-ctl --version >version.txt &&
    test_when_finished "rm version.txt"
'

test_expect_success "cluster-ctl --help and -h succeed" '
    ipfs-cluster-ctl --help &&
    ipfs-cluster-ctl -h 
'

test_expect_success "cluster-ctl help and h succeed" '
    ipfs-cluster-ctl h &&
    ipfs-cluster-ctl help
'
# TODO 80 character limit for help output 

test_expect_success "cluster-ctl help output looks good" '
    ipfs-cluster-ctl --help | egrep -q -i "^(Usage|Commands|Global options)"
'

# TODO don't use an intermediate file.  Is this necessary?
test_expect_success "cluster-ctl commands output looks good" '
    ipfs-cluster-ctl commands | awk '"'"'NF'"'"' >commands.txt &&
    test_when_finished "rm commands.txt" &&
    numCmds=`cat commands.txt | sed '"'"'/^s*$/d'"'"' | wc -l` &&
    [ $numCmds -eq "8" ] &&
    egrep -q "ipfs-cluster-ctl id" commands.txt &&
    egrep -q "ipfs-cluster-ctl peers" commands.txt &&
    egrep -q "ipfs-cluster-ctl pin" commands.txt &&
    egrep -q "ipfs-cluster-ctl status" commands.txt &&
    egrep -q "ipfs-cluster-ctl sync" commands.txt &&
    egrep -q "ipfs-cluster-ctl recover" commands.txt &&
    egrep -q "ipfs-cluster-ctl version" commands.txt &&
    egrep -q "ipfs-cluster-ctl commands" commands.txt
'

# TODO  generalize and put inside of library for use in other tests, specifically with help output 
test_expect_success "All cluster-ctl command docs are 80 columns or less" '
   export failure="0" &&
   ipfs-cluster-ctl commands | awk '"'"'NF'"'"' >commands.txt &&
   test_when_finished "rm commands.txt" &&
   while read cmd
   do
       LENGTH="$($cmd --help | awk '"'"'{ print length }'"'"' | sort -nr | head -n 1)"
       [ "$LENGTH" -gt 80 ] &&
           { echo "$cmd" help text is longer than 79 chars "($LENGTH)"; export failure="1"; }
   done <commands.txt

   if [ $failure -eq "1" ]; then
       return 1
   fi
'
test_done

