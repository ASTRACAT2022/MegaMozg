#!/usr/bin/env bash
set -euo pipefail

TOKEN=""
CORE_URL="${MEZA_CORE_URL:-http://127.0.0.1:8080}"
NODE_NAME="${MEZA_NODE_NAME:-$(hostname | tr '[:upper:]' '[:lower:]')}"
NODE_REGION="${MEZA_NODE_REGION:-unknown-region}"
NODE_IP="${MEZA_NODE_IP:-$(hostname -I 2>/dev/null | awk '{print $1}')}"
TAGS="${MEZA_NODE_TAGS:-}"
HEARTBEAT_INTERVAL="${MEZA_HEARTBEAT_INTERVAL:-15}"
HEARTBEAT_STATUS="${MEZA_HEARTBEAT_STATUS:-online}"
SERVICE_NAME="meza-node-heartbeat"
ENV_PATH="/etc/meza-node/node.env"
BIN_PATH="/usr/local/bin/meza-node-heartbeat"
INSTALL_DIR="${MEZA_NODE_INSTALL_DIR:-/opt/meza-node}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --token)
      TOKEN="${2:-}"
      shift 2
      ;;
    --core-url)
      CORE_URL="${2:-}"
      shift 2
      ;;
    --name)
      NODE_NAME="${2:-}"
      shift 2
      ;;
    --region)
      NODE_REGION="${2:-}"
      shift 2
      ;;
    --tags)
      TAGS="${2:-}"
      shift 2
      ;;
    --interval)
      HEARTBEAT_INTERVAL="${2:-}"
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "${TOKEN}" ]]; then
  echo "missing required --token" >&2
  exit 1
fi

run_as_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required but not installed" >&2
  exit 1
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

echo "Meza-Node installer"
echo "os=${OS}"
echo "arch=${ARCH}"
echo "core_url=${CORE_URL}"
echo "node_name=${NODE_NAME}"
echo "region=${NODE_REGION}"
echo "node_ip=${NODE_IP:-unknown}"
echo "heartbeat_interval=${HEARTBEAT_INTERVAL}s"

if [[ -n "${TAGS}" ]]; then
  TAGS_JSON="$(printf '%s' "${TAGS}" | awk -F',' '{printf "["; for (i=1; i<=NF; i++) {gsub(/^ +| +$/, "", $i); printf "%s\"%s\"", (i>1?",":""), $i} printf "]"}')"
else
  TAGS_JSON="[]"
fi

curl -fsSL -X POST "${CORE_URL}/api/v1/nodes/register" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{\"name\":\"${NODE_NAME}\",\"region\":\"${NODE_REGION}\",\"ip_address\":\"${NODE_IP}\",\"tags\":${TAGS_JSON}}"

TMP_ENV="$(mktemp)"
TMP_BIN="$(mktemp)"
TMP_SERVICE="$(mktemp)"
cleanup() {
  rm -f "${TMP_ENV}" "${TMP_BIN}" "${TMP_SERVICE}"
}
trap cleanup EXIT

cat >"${TMP_ENV}" <<EOF
CORE_URL="${CORE_URL}"
AUTH_TOKEN="${TOKEN}"
NODE_NAME="${NODE_NAME}"
NODE_REGION="${NODE_REGION}"
NODE_IP="${NODE_IP}"
TAGS_JSON='${TAGS_JSON}'
HEARTBEAT_STATUS="${HEARTBEAT_STATUS}"
HEARTBEAT_INTERVAL="${HEARTBEAT_INTERVAL}"
EOF

cat >"${TMP_BIN}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="/etc/meza-node/node.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE}" >&2
  exit 1
fi

# shellcheck disable=SC1090
source "${ENV_FILE}"

INTERVAL="${HEARTBEAT_INTERVAL:-15}"
STATUS="${HEARTBEAT_STATUS:-online}"

while true; do
  curl -fsS -X POST "${CORE_URL}/api/v1/nodes/heartbeat" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${AUTH_TOKEN}" \
    -d "{\"name\":\"${NODE_NAME}\",\"region\":\"${NODE_REGION}\",\"ip_address\":\"${NODE_IP}\",\"tags\":${TAGS_JSON},\"status\":\"${STATUS}\",\"metrics\":{\"cpu_percent\":0,\"ram_percent\":0,\"disk_percent\":0,\"network_kbps\":0,\"load_average\":0,\"processes_count\":0}}" \
    >/dev/null || true
  sleep "${INTERVAL}"
done
EOF

run_as_root mkdir -p "$(dirname "${ENV_PATH}")" "${INSTALL_DIR}"
run_as_root install -m 600 "${TMP_ENV}" "${ENV_PATH}"
run_as_root install -m 755 "${TMP_BIN}" "${BIN_PATH}"

if command -v systemctl >/dev/null 2>&1; then
  cat >"${TMP_SERVICE}" <<EOF
[Unit]
Description=Meza-Node heartbeat agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_PATH}
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  run_as_root install -m 644 "${TMP_SERVICE}" "/etc/systemd/system/${SERVICE_NAME}.service"
  run_as_root systemctl daemon-reload
  run_as_root systemctl enable --now "${SERVICE_NAME}.service"
  SERVICE_MODE="systemd"
else
  LOG_FILE="${INSTALL_DIR}/heartbeat.log"
  PID_FILE="${INSTALL_DIR}/heartbeat.pid"
  run_as_root sh -c "nohup '${BIN_PATH}' >> '${LOG_FILE}' 2>&1 & echo \$! > '${PID_FILE}'"
  SERVICE_MODE="background"
fi

curl -fsSL -X POST "${CORE_URL}/api/v1/nodes/heartbeat" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{\"name\":\"${NODE_NAME}\",\"region\":\"${NODE_REGION}\",\"ip_address\":\"${NODE_IP}\",\"tags\":${TAGS_JSON},\"status\":\"${HEARTBEAT_STATUS}\",\"metrics\":{\"cpu_percent\":0,\"ram_percent\":0,\"disk_percent\":0,\"network_kbps\":0,\"load_average\":0,\"processes_count\":0}}" \
  >/dev/null || true

echo
echo "Node registered and connected automatically."
echo "Heartbeat service mode: ${SERVICE_MODE}"
echo "No extra clicks required. The node now sends heartbeat to the hub."
