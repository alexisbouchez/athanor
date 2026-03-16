#!/bin/bash
set -euo pipefail

# Setup script for the Athanor CI GitHub App.
# Run after creating the app at github.com/settings/apps/new
#
# Usage: ./scripts/setup-github-app.sh <APP_ID> <PRIVATE_KEY_PEM_PATH>

if [ $# -lt 2 ]; then
    echo "Usage: $0 <APP_ID> <PRIVATE_KEY_PEM_PATH>"
    echo ""
    echo "Steps:"
    echo "  1. Go to https://github.com/settings/apps/new"
    echo "  2. Set name: athanor-ci"
    echo "  3. Set homepage: http://137.74.44.90:8080"
    echo "  4. UNCHECK 'Webhook > Active'"
    echo "  5. Permissions: Checks=Read&Write, Statuses=Read&Write, Contents=Read, Metadata=Read"
    echo "  6. Click 'Create GitHub App'"
    echo "  7. Note the App ID from the app settings page"
    echo "  8. Click 'Generate a private key' — downloads a .pem file"
    echo "  9. Click 'Install App' in the sidebar, install on your account for the athanor repo"
    echo " 10. Note the Installation ID from the URL (github.com/settings/installations/<ID>)"
    echo " 11. Run: $0 <APP_ID> <path-to-downloaded.pem>"
    exit 1
fi

APP_ID="$1"
PEM_PATH="$2"
VPS="root@vps-8849e3c3.vps.ovh.net"

echo "App ID: $APP_ID"
echo "PEM: $PEM_PATH"

# Find installation ID
echo "Looking up installation ID..."
# Generate a JWT to query the API
JWT=$(python3 -c "
import jwt, time, sys
with open('$PEM_PATH', 'r') as f:
    key = f.read()
now = int(time.time())
payload = {'iat': now - 60, 'exp': now + 600, 'iss': '$APP_ID'}
print(jwt.encode(payload, key, algorithm='RS256'))
" 2>/dev/null) || true

if [ -n "$JWT" ]; then
    INSTALLATION_ID=$(curl -s \
        -H "Authorization: Bearer $JWT" \
        -H "Accept: application/vnd.github+json" \
        https://api.github.com/app/installations \
        | python3 -c "import json,sys; data=json.load(sys.stdin); print(data[0]['id'])" 2>/dev/null) || true
fi

if [ -z "${INSTALLATION_ID:-}" ]; then
    echo "Could not auto-detect installation ID."
    echo "Please enter it (from https://github.com/settings/installations/<ID>):"
    read -r INSTALLATION_ID
fi

echo "Installation ID: $INSTALLATION_ID"

# Upload PEM to VPS
echo "Uploading private key to VPS..."
scp "$PEM_PATH" "$VPS:/etc/athanor/github-app.pem"
ssh "$VPS" "chmod 600 /etc/athanor/github-app.pem"

# Update env file
echo "Updating server config..."
ssh "$VPS" "cat >> /etc/athanor/env << EOF
GITHUB_APP_ID=$APP_ID
GITHUB_APP_INSTALLATION_ID=$INSTALLATION_ID
GITHUB_APP_PRIVATE_KEY_PATH=/etc/athanor/github-app.pem
EOF"

# Restart
echo "Restarting athanor..."
ssh "$VPS" "systemctl restart athanor"
sleep 2
ssh "$VPS" "journalctl -u athanor -n 5 --no-pager"

echo ""
echo "Done! GitHub App configured."
echo "Push to the repo to test — check runs should now appear with full logs."
