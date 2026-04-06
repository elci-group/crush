#!/bin/sh
set -e

echo "Building crush..."
go build -o crush .
echo "Build complete."