#!/bin/sh
# Auto-detect the GIDs of mounted resources (Docker socket, systemd journal)
# so the agent can access them without requiring the operator to pre-configure
# host group IDs. Drops from root to PUID/PGID (default 65532) before exec.
set -e

# Runtime identity. Set PUID/PGID to your host user's IDs when you own the
# watched paths — no host permission changes are needed in that case.
# Defaults to 65532 (distroless nonroot).
#
# validate_id VALUE DEFAULT LABEL — returns DEFAULT (with a warning) if VALUE
# is empty, non-numeric, or zero (root). Prevents accidentally running as root
# or passing garbage to setpriv.
validate_id() {
    val="$1" default="$2" label="$3"
    case "$val" in
        '')
            printf '%s' "$default"; return ;;
        *[!0-9]*)
            printf 'entrypoint: %s="%s" is not a valid integer; using default %s\n' \
                "$label" "$val" "$default" >&2
            printf '%s' "$default"; return ;;
    esac
    if [ "$val" -eq 0 ]; then
        printf 'entrypoint: %s=0 would run as root; using default %s\n' \
            "$label" "$default" >&2
        printf '%s' "$default"; return
    fi
    printf '%s' "$val"
}

TARGET_UID=$(validate_id "${PUID:-}" 65532 PUID)
TARGET_GID=$(validate_id "${PGID:-}" 65532 PGID)

# Helper function to add unique GID to GROUPS_ARG
add_unique_gid() {
    candidate=$(printf '%s' "$1" | tr -d '[:space:]')
    if [ -z "$candidate" ]; then
        return 0
    fi

    if [ -z "$GROUPS_ARG" ]; then
        GROUPS_ARG="$candidate"
    elif ! echo "$GROUPS_ARG" | grep -qE "(^|,)$candidate(,|$)"; then
        GROUPS_ARG="$GROUPS_ARG,$candidate"
    fi
}

if [ -e /var/run/docker.sock ]; then
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)
    add_unique_gid "$DOCKER_GID"
fi

# Check both common journal paths and add the first one found
if [ -d /run/log/journal ]; then
    JOURNAL_GID=$(stat -c '%g' /run/log/journal)
    add_unique_gid "$JOURNAL_GID"
elif [ -d /var/log/journal ]; then
    JOURNAL_GID=$(stat -c '%g' /var/log/journal)
    add_unique_gid "$JOURNAL_GID"
fi

if [ -n "$GROUPS_ARG" ]; then
    exec setpriv --reuid="$TARGET_UID" --regid="$TARGET_GID" --groups="$GROUPS_ARG" /blackbox-agent "$@"
else
    exec setpriv --reuid="$TARGET_UID" --regid="$TARGET_GID" --clear-groups /blackbox-agent "$@"
fi
