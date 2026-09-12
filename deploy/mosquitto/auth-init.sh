#!/bin/sh
# Bootstrap broker auth for OCM IoT (see iot/spec/spec.md §8 and
# deploy/mosquitto/README.md). Runs as the entrypoint of the one-shot
# mosquitto-init container before the broker starts.
#
# Idempotent: source passwords already present in credentials.txt are reused
# so container restarts never rotate them; a source added to IOT_SOURCES gets
# a fresh random password appended. To rotate everything, clear credentials.txt
# in the mosquitto_auth volume (or delete the volume).
set -eu

AUTH_DIR=/mosquitto-config/auth
BACKEND_USER="${IOT_MQTT_USERNAME:-ocm-backend}"
BACKEND_PASS="${IOT_MQTT_PASSWORD:?set IOT_MQTT_PASSWORD (backend broker credential)}"
SITE="${IOT_SITE_ID:-main}"
SOURCES="${IOT_SOURCES:-}"
CRED_FILE="$AUTH_DIR/credentials.txt"

mkdir -p "$AUTH_DIR"
touch "$CRED_FILE"

# ACL: the backend user may read/write the whole namespace; every source is
# scoped to its own subtree (state/event/ack publish, cmd subscribe, will).
{
  echo "user $BACKEND_USER"
  echo "topic readwrite iot/#"
} > "$AUTH_DIR/acl"

# The password file is rebuilt from scratch on every run.
: > "$AUTH_DIR/passwd"
mosquitto_passwd -b -c "$AUTH_DIR/passwd" "$BACKEND_USER" "$BACKEND_PASS"

get_pass() {
  grep "^$1 " "$CRED_FILE" 2>/dev/null | head -n 1 | cut -d ' ' -f 2
}

gen_pass() {
  head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 24
}

for src in $SOURCES; do
  case "$src" in
    *[!A-Za-z0-9._-]* | '')
      echo "auth-init: invalid source id: $src" >&2
      exit 1
      ;;
  esac
  user="src-$src"
  pass="$(get_pass "$user")"
  if [ -z "$pass" ]; then
    pass="$(gen_pass)"
    echo "$user $pass" >> "$CRED_FILE"
  fi
  mosquitto_passwd -b "$AUTH_DIR/passwd" "$user" "$pass"
  {
    echo ""
    echo "user $user"
    echo "topic write iot/$SITE/$src/state"
    echo "topic write iot/$SITE/$src/event"
    echo "topic write iot/$SITE/$src/ack"
    echo "topic read iot/$SITE/$src/+/cmd"
    echo "topic write iot/_meta/$src/offline"
  } >> "$AUTH_DIR/acl"
  echo "auth-init: broker credential ready for source '$src' ($user)"
done

echo "auth-init: complete (backend=$BACKEND_USER; sources: ${SOURCES:-none})"
echo "auth-init: source credentials at $CRED_FILE inside the mosquitto_auth volume"
