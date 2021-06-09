#!/usr/bin/env bash
set -euo pipefail

if ! uplink &> /dev/null; then
    echo "uplink command not found"
    exit 1
fi
if [ -z "$ACCESS_GRANT" ]; then
    echo "ACCESS_GRANT env var not defined"
    exit 1
fi
if [ $# -eq 0 ]; then
    echo "usage: $0 <BUCKET> <CONCURRENCY_LIMIT>"
    exit 1
fi
if [ -z "$1" ]; then
    echo "Bucket name not provided"
    exit 1
fi

BUCKET=$1
CONCURRENCY_LIMIT=${2:-100} # second argument, default to 100
AUTH_URL="https://auth.us1.storjshare.io"
BASE_URL="https://link.us1.storjshare.io"
EXPIRE_AFTER="+2h"

echo "Testing using files in sj://$BUCKET"
echo "Concurrency limit set to $CONCURRENCY_LIMIT"

FILES=$(uplink --access "$ACCESS_GRANT" ls "sj://$BUCKET" | awk '{ print $5 }')
if [ -z "$FILES" ]; then
    echo "No files found"
    exit 1
fi

SHARE_URL=$(uplink share --access "$ACCESS_GRANT" \
    --auth-service "$AUTH_URL" \
    --base-url "$BASE_URL" \
    --not-after "$EXPIRE_AFTER" \
    --readonly \
    --url \
    "sj://$BUCKET" | grep -e "^URL\s*:" | awk '{ print $3 }')

URLS_FILE=$(mktemp /tmp/testXXXXXXX)
trap 'rm "$URLS_FILE"' EXIT

IFS=$'\n'
for FILE in $FILES; do
    URL="${SHARE_URL}${FILE}?download=1"
    printf "url=%s\noutput=/dev/null\n" "$URL" >> "$URLS_FILE"
done

curl \
    --parallel \
    --parallel-immediate \
    --parallel-max "$CONCURRENCY_LIMIT" \
    --config "$URLS_FILE"

echo "Done"