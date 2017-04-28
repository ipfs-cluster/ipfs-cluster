#!/bin/sh
#
# MIT License

# Run tests 
cd "$(dirname "$0")"
for i in `ls t*.sh | sort`;
do
    echo "*** $i ***"
    ./$i 
done

# Aggregate Results
echo "We are aggregating"
for f in test-results/*.counts; do
    echo "$f";
done | bash aggregate-results.sh
