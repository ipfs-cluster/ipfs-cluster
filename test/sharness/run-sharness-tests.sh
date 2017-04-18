#!/bin/sh
#
# MIT License
cd "$(dirname "$0")"
for i in `ls t*.sh | sort`;
do
    echo "*** $i ***"
    ./$i 
done
