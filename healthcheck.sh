#!/bin/sh

# Health check script for Go Forum
# Usage: ./healthcheck.sh [URL]

URL=${1:-http://localhost:8080}

# Check if curl is available, if not use wget
if command -v curl >/dev/null 2>&1; then
    response=$(curl -s -o /dev/null -w "%{http_code}" "$URL")
    if [ "$response" = "200" ]; then
        echo "Forum is healthy (HTTP $response)"
        exit 0
    else
        echo "Forum is unhealthy (HTTP $response)"
        exit 1
    fi
elif command -v wget >/dev/null 2>&1; then
    if wget --quiet --tries=1 --spider "$URL"; then
        echo "Forum is healthy"
        exit 0
    else
        echo "Forum is unhealthy"
        exit 1
    fi
else
    echo "Neither curl nor wget is available for health check"
    exit 1
fi