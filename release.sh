#!/bin/bash

# Updates the Version variables, commits, tags and signs

set -e
set -x

version="$1"

if [ -z $version ]; then
   echo "Need a version!"
   exit 1  
fi

make clean
sed -i "s/Version = semver\.MustParse.*$/Version = semver.MustParse(\"$version\")/" version/version.go
sed -i "s/const Version.*$/const Version = \"$version\"/" cmd/ipfs-cluster-ctl/main.go
git commit -S -a -m "Release $version"
lastver=`git tag -l | grep -E 'v[0-9]+\.[0-9]+\.[0-9]+$' | tail -n 1`
echo "Tag for Release ${version}" > tag_annotation
echo >> tag_annotation
git log --pretty=oneline ${lastver}..HEAD >> tag_annotation
git tag -a -s -F tag_annotation v$version
rm tag_annotation