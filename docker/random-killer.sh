#! /bin/bash

# $1=sleep time, $2=first command, $3=second command
# Loop forever
KILLED=1
sleep 2
while true; do
    if [ "$(($RANDOM % 10))" -gt "4" ]; then
        # Take down cluster
        if [ "$KILLED" -eq "1" ]; then
            KILLED=0
            killall ipfs-cluster-service
        else # Bring up cluster
            KILLED=1
            ipfs-cluster-service &
        fi
    fi
    sleep 2.5
done
