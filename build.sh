#!/bin/bash
export GOARCH=amd64
fn="${1:-tsalign}"
filename=$(basename "$fn")
extension="${filename##*.}"
filename="${filename%.*}"
env GOOS=windows go build -o "$filename"_win-"$GOARCH".exe "$1"
env GOOS=linux go build -o "$filename"_linux-"$GOARCH" "$1"
env GOOS=darwin go build -o "$filename"_darwin-"$GOARCH" "$1"
