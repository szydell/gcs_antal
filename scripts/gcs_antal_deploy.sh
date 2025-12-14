#!/usr/bin/env bash
set -euo pipefail

# Oczekiwane argumenty:
# 1) ścieżka źródłowa (tmp)
# 2) ścieżka docelowa (binarka)
SRC="${1:?missing SRC}"
DST="${2:?missing DST}"

# Twarde ograniczenia bezpieczeństwa:
SERVICE="gcs_antal"
case "$SRC" in
  /tmp/gcs_antal-*-amd64)
    ALLOWED_DST="/usr/local/bin/gcs_antal-linux-amd64"
    ;;
  /tmp/gcs_antal-*-arm64)
    ALLOWED_DST="/usr/local/bin/gcs_antal-linux-arm64"
    ;;
  *)
    echo "SRC not allowed: $SRC" >&2
    exit 2
    ;;
esac

if [[ "$DST" != "$ALLOWED_DST" ]]; then
  echo "DST not allowed: $DST" >&2
  exit 3
fi

install -m 0755 "$SRC" "$DST"
systemctl restart "$SERVICE"
rm -f "$SRC"