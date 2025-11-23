#!/bin/bash

# Get minor version (number of git commits)
MINOR_VERSION=$(git rev-list --count HEAD)

# Get short git commit hash
GIT_COMMIT=$(git rev-parse --short HEAD)

# Major version is hardcoded as '0'
MAJOR_VERSION="0"

# Build the Go program with version information injected using ldflags
go build -o 0s -ldflags "-X main.majorVersion=${MAJOR_VERSION} -X main.minorVersion=${MINOR_VERSION} -X main.gitCommit=${GIT_COMMIT}"

if [ $? -eq 0 ]; then
    echo "Build successful: 0s version ${MAJOR_VERSION}.${MINOR_VERSION}-${GIT_COMMIT}"
else
    echo "Build failed."
    exit 1
fi