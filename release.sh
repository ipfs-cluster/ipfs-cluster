#!/bin/bash

# Updates the Version variables, commits, tags and signs

set -eux

version="$1"

if [ -z $version ]; then
   echo "Need a version!"
   exit 1  
fi

make clean
sed -i "s/Version = semver\.MustParse.*$/Version = semver.MustParse(\"$version\")/" version/version.go
sed -i "s/const Version.*$/const Version = \"$version\"/" cmd/ipfs-cluster-ctl/main.go
git add version/version.go cmd/ipfs-cluster-ctl/main.go

# Next versions, just commit
if [[ "$version" == *"-next" ]]; then
    git commit -S -m "Set development version v${version}"
    exit 0
fi

# RC versions, commit and make a non-annotated tag.
if [[ "$version" == *"-rc"* ]]; then
    git commit -S -m "Release candidate v${version}"
    git tag -s "v${version}"
    exit 0
fi

# Actual releases, commit and make an annotated tag with all the commits
# since the last.
git commit -S -m "Release v${version}"
lastver=`git describe --abrev=0`
echo "Tag for Release ${version}" > tag_annotation
echo >> tag_annotation
git log --pretty=oneline ${lastver}..HEAD >> tag_annotation
git tag -a -s -F tag_annotation "v${version}"
rm tag_annotation
