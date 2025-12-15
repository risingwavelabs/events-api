#!/bin/bash
set -e

_os=$(uname -s)

if [ "$_os" != "Linux" ] && [ "$_os" != "Darwin" ]; then
	echo "Unsupported OS: $_os"
	exit 1
fi

_arch=$(uname -m)

if [ "$_arch" != "x86_64" ] && [ "$_arch" != "i386" ] && [ "$_arch" != "arm64" ]; then
	echo "Unsupported arch: $_arch"
	exit 1
fi

if ! [ -x "$(command -v curl)" ]; then
	echo "command 'curl' not found."
	exit 1
fi

# check if the binary exists
download_url="https://rwtools.s3.ap-southeast-1.amazonaws.com/eventapi/$_os/$_arch/eventapi"
status_code=$(curl -s -o /dev/null -I -w '%{http_code}' "$download_url")

echo "$download_url"

if [ "$status_code" != 200 ]; then
	echo "Error $status_code: failed to install eventapi."
	exit 1
fi

# download
curl -L -o ./eventapi "$download_url"
chmod 755 ./eventapi

echo "eventapi installed successfully."
