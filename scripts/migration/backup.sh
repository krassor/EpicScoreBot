#!/bin/bash
# Backup script for EpicScoreBot PostgreSQL database
# Usage: ./backup.sh [output_path]

set -euo pipefail

CONTAINER_NAME="postgres"
DB_USER="epicbot"
DB_NAME="epic-score-db"
BACKUP_FILENAME="epic-score-db-dump.bak"
CONTAINER_TEMP_PATH="/var/lib/postgresql/${BACKUP_FILENAME}"

OUTPUT_PATH="${1:-./${BACKUP_FILENAME}}"

echo "=== Starting database backup ==="

# 1. Check if PostgreSQL container is running
if ! docker ps --filter "name=^/${CONTAINER_NAME}$" --filter "status=running" | grep -q "${CONTAINER_NAME}"; then
  echo "Error: Container '${CONTAINER_NAME}' is not running." >&2
  exit 1
fi

# 2. Run pg_dump inside the container
echo "Running pg_dump inside the container..."
docker exec -t "${CONTAINER_NAME}" pg_dump \
  -U "${DB_USER}" \
  -d "${DB_NAME}" \
  -F c \
  -b \
  -v \
  -f "${CONTAINER_TEMP_PATH}"

# 3. Copy backup file from container to host
echo "Copying backup from container to host: ${OUTPUT_PATH}"
docker cp "${CONTAINER_NAME}:${CONTAINER_TEMP_PATH}" "${OUTPUT_PATH}"

# 4. Clean up temporary file in container
echo "Cleaning up temporary files inside the container..."
docker exec -it "${CONTAINER_NAME}" rm "${CONTAINER_TEMP_PATH}"

echo "=== Backup completed successfully ==="
echo "Backup file is saved at: ${OUTPUT_PATH}"
echo "You can now transfer this file to the target server using scp or rsync."
echo "Example: scp ${OUTPUT_PATH} user@target-server-ip:/tmp/"
