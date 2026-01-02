#!/bin/bash
# ============================================================
# llm-mux config migration script
# Migrates old config format to new unified format
#
# Usage:
#   ./scripts/migrate-config.sh [config_path]
#
# If no path provided, uses default XDG config location
# ============================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()   { echo -e "${GREEN}==>${NC} $*"; }
info()  { echo -e "   $*"; }
warn()  { echo -e "${YELLOW}warning:${NC} $*" >&2; }
error() { echo -e "${RED}error:${NC} $*" >&2; exit 1; }

CONFIG_PATH="${1:-${XDG_CONFIG_HOME:-$HOME/.config}/llm-mux/config.yaml}"

if [[ ! -f "$CONFIG_PATH" ]]; then
    info "No config file found at $CONFIG_PATH, skipping migration"
    exit 0
fi

needs_migration() {
    grep -qE '^usage-statistics-enabled:|^usage-persistence:' "$CONFIG_PATH" 2>/dev/null
}

if ! needs_migration; then
    info "Config already in new format, no migration needed"
    exit 0
fi

log "Migrating config: $CONFIG_PATH"

BACKUP_PATH="${CONFIG_PATH}.backup.$(date +%Y%m%d%H%M%S)"
cp "$CONFIG_PATH" "$BACKUP_PATH"
info "Backup created: $BACKUP_PATH"

OLD_ENABLED=""
OLD_PERSISTENCE_ENABLED=""
OLD_DB_PATH=""
OLD_BATCH_SIZE=""
OLD_FLUSH_INTERVAL=""
OLD_RETENTION_DAYS=""

if grep -q '^usage-statistics-enabled:' "$CONFIG_PATH"; then
    OLD_ENABLED=$(grep '^usage-statistics-enabled:' "$CONFIG_PATH" | sed 's/usage-statistics-enabled:[[:space:]]*//' | tr -d ' ')
fi

if grep -q '^usage-persistence:' "$CONFIG_PATH"; then
    in_persistence=false
    while IFS= read -r line; do
        if [[ "$line" =~ ^usage-persistence: ]]; then
            in_persistence=true
            continue
        fi
        if [[ "$in_persistence" == true ]]; then
            if [[ "$line" =~ ^[a-z] && ! "$line" =~ ^[[:space:]] ]]; then
                break
            fi
            
            key=$(echo "$line" | sed 's/^[[:space:]]*//' | cut -d: -f1)
            value=$(echo "$line" | sed 's/^[^:]*:[[:space:]]*//' | sed 's/^"//' | sed 's/"$//' | tr -d ' ')
            
            case "$key" in
                enabled)         OLD_PERSISTENCE_ENABLED="$value" ;;
                db-path)         OLD_DB_PATH="$value" ;;
                batch-size)      OLD_BATCH_SIZE="$value" ;;
                flush-interval)  OLD_FLUSH_INTERVAL="$value" ;;
                retention-days)  OLD_RETENTION_DAYS="$value" ;;
            esac
        fi
    done < "$CONFIG_PATH"
fi

NEW_DSN=""
if [[ "$OLD_ENABLED" == "true" || "$OLD_PERSISTENCE_ENABLED" == "true" ]]; then
    if [[ -n "$OLD_DB_PATH" ]]; then
        DB_PATH_EXPANDED="${OLD_DB_PATH/#\~/$HOME}"
        NEW_DSN="sqlite://$DB_PATH_EXPANDED"
    else
        NEW_DSN="sqlite://${XDG_CONFIG_HOME:-$HOME/.config}/llm-mux/usage.db"
    fi
fi

NEW_BATCH_SIZE="${OLD_BATCH_SIZE:-100}"
NEW_RETENTION="${OLD_RETENTION_DAYS:-30}"

if [[ -n "$OLD_FLUSH_INTERVAL" && "$OLD_FLUSH_INTERVAL" =~ ^[0-9]+$ ]]; then
    NEW_FLUSH="${OLD_FLUSH_INTERVAL}s"
else
    NEW_FLUSH="5s"
fi

TMP_FILE=$(mktemp)
trap 'rm -f "$TMP_FILE"' EXIT

in_old_section=false
skip_until_next_key=false

while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^usage-statistics-enabled: ]]; then
        continue
    fi
    
    if [[ "$line" =~ ^usage-persistence: ]]; then
        skip_until_next_key=true
        continue
    fi
    
    if [[ "$skip_until_next_key" == true ]]; then
        if [[ "$line" =~ ^[a-z] && ! "$line" =~ ^[[:space:]] ]]; then
            skip_until_next_key=false
        else
            continue
        fi
    fi
    
    echo "$line" >> "$TMP_FILE"
done < "$CONFIG_PATH"

if [[ -n "$NEW_DSN" ]]; then
    cat >> "$TMP_FILE" << EOF

usage:
  dsn: "$NEW_DSN"
  batch-size: $NEW_BATCH_SIZE
  flush-interval: "$NEW_FLUSH"
  retention-days: $NEW_RETENTION
EOF
fi

mv "$TMP_FILE" "$CONFIG_PATH"

log "Migration complete!"
info "Old format (removed):"
info "  - usage-statistics-enabled"
info "  - usage-persistence.enabled"
info "  - usage-persistence.db-path"
info ""
info "New format (added):"
info "  - usage.dsn: $NEW_DSN"
info "  - usage.batch-size: $NEW_BATCH_SIZE"
info "  - usage.flush-interval: $NEW_FLUSH"
info "  - usage.retention-days: $NEW_RETENTION"
