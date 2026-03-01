#!/usr/bin/env bash
set -euo pipefail

# Initialize Garage: configure node layout, create bucket and API key.
# Run this once after the first `docker compose up`.

COMPOSE="docker compose"

echo "Waiting for Garage to be ready..."
for i in $(seq 1 30); do
    if $COMPOSE exec garage /garage status &>/dev/null; then
        echo "Garage is ready."
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "ERROR: Garage did not become ready in time."
        exit 1
    fi
    sleep 1
done

# Get node ID
NODE_ID=$($COMPOSE exec garage /garage status 2>/dev/null | grep -oP '[a-f0-9]{16}' | head -1)
if [ -z "$NODE_ID" ]; then
    echo "ERROR: Could not determine Garage node ID."
    exit 1
fi
echo "Node ID: $NODE_ID"

# Assign layout (1 GB capacity, zone dc1)
echo "Assigning layout..."
$COMPOSE exec garage /garage layout assign -z dc1 -c 1G "$NODE_ID"

# Apply layout
LAYOUT_VERSION=$($COMPOSE exec garage /garage layout show 2>/dev/null | grep -oP 'version \K[0-9]+' || echo "1")
$COMPOSE exec garage /garage layout apply --version "$LAYOUT_VERSION"

echo "Layout applied."

# Create bucket
echo "Creating bucket skadia-originals..."
$COMPOSE exec garage /garage bucket create skadia-originals 2>/dev/null || echo "Bucket already exists."

# Create API key
echo "Creating API key..."
$COMPOSE exec garage /garage key create skadia-dev-key 2>/dev/null || echo "Key already exists."

# Allow access
echo "Granting access..."
$COMPOSE exec garage /garage bucket allow skadia-originals --read --write --key skadia-dev-key

echo ""
echo "Garage initialization complete."
echo "Key info:"
$COMPOSE exec garage /garage key info skadia-dev-key
