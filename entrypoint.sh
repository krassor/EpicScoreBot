#!/bin/sh

echo "--- Starting Entrypoint Script ---"

# Create necessary directories
mkdir -p ${CONFIG_FILEPATH}

echo "--- Dirs created. Listing contents of VOLUME_PATH: ---"
ls -la ${VOLUME_PATH}

# Start the Go application
exec /bin/epicScoreBot "$@"
