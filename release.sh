#!/bin/sh
# Usage: ./release.sh v0.0.7 "release notes"
# Builds cs2-demo-<version>.exe, tags the current commit and publishes a GitHub release.
set -eu
version=$1
exe="cs2-demo-$version.exe"
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o "$exe" .
git tag "$version"
git push origin "$version"
gh release create "$version" "$exe" --title "$version" --notes "$2"
