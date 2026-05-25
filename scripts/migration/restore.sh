#!/bin/bash
# Restore script for EpicScoreBot PostgreSQL database
# Usage: ./restore.sh <path_to_backup_file>

set -euo pipefail

CONTAINER_NAME="postgres"
DB_USER="epicbot"
DB_NAME="epic-score-db"
BACKUP_FILENAME="epic-score-db-dump.bak"
CONTAINER_TEMP_PATH="/var/lib/postgresql/${BACKUP_FILENAME}"

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <path_to_backup_file>" >&2
  exit 1
fi

INPUT_PATH="$1"

if [ ! -f "${INPUT_PATH}" ]; then
  echo "Error: Backup file '${INPUT_PATH}' does not exist." >&2
  exit 1
fi

echo "=== Starting database restore ==="

# 1. Check if PostgreSQL container is running
if ! docker ps --filter "name=^/${CONTAINER_NAME}$" --filter "status=running" | grep -q "${CONTAINER_NAME}"; then
  echo "Error: Container '${CONTAINER_NAME}' is not running." >&2
  exit 1
fi

# 2. Copy backup file from host into container
echo "Copying backup from host into container..."
docker cp "${INPUT_PATH}" "${CONTAINER_NAME}:${CONTAINER_TEMP_PATH}"

# 3. Restore database schema and data
echo "Restoring database using pg_restore..."
docker exec -it "${CONTAINER_NAME}" pg_restore \
  -U "${DB_USER}" \
  -d "${DB_NAME}" \
  -v \
  --clean \
  --no-owner \
  --no-privileges \
  "${CONTAINER_TEMP_PATH}"

# 4. Clean up temporary file in container
echo "Cleaning up temporary files inside the container..."
docker exec -it "${CONTAINER_NAME}" rm "${CONTAINER_TEMP_PATH}"

echo "=== Restore completed successfully ==="
