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
TERMINAL_POLL_INTERVAL="${MEZA_TERMINAL_POLL_INTERVAL:-2}"
SERVICE_NAME="meza-node-heartbeat"
EXEC_SERVICE_NAME="meza-node-exec"
ENV_PATH="/etc/meza-node/node.env"
BIN_PATH="/usr/local/bin/meza-node-heartbeat"
EXEC_BIN_PATH="/usr/local/bin/meza-node-exec"
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
TMP_EXEC_BIN="$(mktemp)"
TMP_EXEC_SERVICE="$(mktemp)"
cleanup() {
  rm -f "${TMP_ENV}" "${TMP_BIN}" "${TMP_SERVICE}" "${TMP_EXEC_BIN}" "${TMP_EXEC_SERVICE}"
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
TERMINAL_POLL_INTERVAL="${TERMINAL_POLL_INTERVAL}"
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
NET_PREV_BYTES=0
NET_PREV_TS=0

log() {
  echo "[meza-node-heartbeat] $*" >&2
}

read_cpu_percent() {
  if [[ ! -r /proc/stat ]]; then
    echo 0
    return 0
  fi

  # shellcheck disable=SC2207
  local cpu_a=($(awk '/^cpu /{print $2, $3, $4, $5, $6, $7, $8, $9, $10}' /proc/stat 2>/dev/null || true))
  if (( ${#cpu_a[@]} == 0 )); then
    echo 0
    return 0
  fi
  local idle_a="${cpu_a[3]:-0}"
  local total_a=0
  local value
  for value in "${cpu_a[@]}"; do
    total_a=$((total_a + ${value:-0}))
  done

  sleep 0.5

  # shellcheck disable=SC2207
  local cpu_b=($(awk '/^cpu /{print $2, $3, $4, $5, $6, $7, $8, $9, $10}' /proc/stat 2>/dev/null || true))
  if (( ${#cpu_b[@]} == 0 )); then
    echo 0
    return 0
  fi
  local idle_b="${cpu_b[3]:-0}"
  local total_b=0
  for value in "${cpu_b[@]}"; do
    total_b=$((total_b + ${value:-0}))
  done

  local total_delta=$((total_b - total_a))
  local idle_delta=$((idle_b - idle_a))
  if (( total_delta <= 0 )); then
    echo 0
    return 0
  fi

  local busy=$((total_delta - idle_delta))
  local cpu_percent=$((busy * 100 / total_delta))
  if (( cpu_percent < 0 )); then cpu_percent=0; fi
  if (( cpu_percent > 100 )); then cpu_percent=100; fi
  echo "${cpu_percent}"
  return 0
}

read_ram_percent() {
  if [[ ! -r /proc/meminfo ]]; then
    echo 0
    return 0
  fi

  local total available used
  total="$(awk '/^MemTotal:/{print $2}' /proc/meminfo 2>/dev/null || true)"
  available="$(awk '/^MemAvailable:/{print $2}' /proc/meminfo 2>/dev/null || true)"
  if [[ -z "${total}" || -z "${available}" || "${total}" -le 0 ]]; then
    echo 0
    return 0
  fi

  used=$((total - available))
  if (( used < 0 )); then used=0; fi
  echo $((used * 100 / total))
  return 0
}

read_disk_percent() {
  local disk
  disk="$(df -P / 2>/dev/null | awk 'NR==2{gsub("%","",$5); print $5}' 2>/dev/null || true)"
  if [[ -z "${disk}" ]]; then
    echo 0
    return 0
  fi
  echo "${disk}"
  return 0
}

read_network_kbps() {
  if [[ ! -r /proc/net/dev ]]; then
    echo 0
    return 0
  fi

  local now total_bytes line iface rest rx tx delta_bytes delta_time kbps
  local -a fields
  now="$(date +%s)"
  total_bytes=0
  while IFS= read -r line; do
    [[ "${line}" != *:* ]] && continue
    iface="${line%%:*}"
    iface="${iface// /}"
    [[ "${iface}" == "lo" ]] && continue

    rest="${line#*:}"
    # Fields in /proc/net/dev after ":" are:
    # rx_bytes rx_packets rx_errs rx_drop rx_fifo rx_frame rx_compressed rx_multicast tx_bytes ...
    read -r -a fields <<<"${rest}"
    rx="${fields[0]:-0}"
    tx="${fields[8]:-0}"
    total_bytes=$((total_bytes + rx + tx))
  done </proc/net/dev

  if (( NET_PREV_TS == 0 || NET_PREV_BYTES == 0 )); then
    NET_PREV_TS="${now}"
    NET_PREV_BYTES="${total_bytes}"
    echo 0
    return 0
  fi

  delta_time=$((now - NET_PREV_TS))
  delta_bytes=$((total_bytes - NET_PREV_BYTES))
  NET_PREV_TS="${now}"
  NET_PREV_BYTES="${total_bytes}"

  if (( delta_time <= 0 || delta_bytes < 0 )); then
    echo 0
    return 0
  fi

  kbps=$(((delta_bytes * 8) / 1000 / delta_time))
  if (( kbps < 0 )); then kbps=0; fi
  echo "${kbps}"
  return 0
}

read_load_average() {
  local load
  load="$(awk '{print $1}' /proc/loadavg 2>/dev/null || true)"
  if [[ -z "${load}" ]]; then
    echo 0
    return 0
  fi
  awk -v v="${load}" 'BEGIN {printf "%d\n", v+0}'
  return 0
}

read_process_count() {
  local output count
  if ! output="$(ps ax 2>/dev/null || true)"; then
    echo 0
    return 0
  fi

  count="$(printf "%s\n" "${output}" | awk 'NR>1{c++} END {print c+0}' 2>/dev/null || true)"
  if [[ -z "${count}" ]]; then
    echo 0
    return 0
  fi

  echo "${count}"
  return 0
}

safe_metric() {
  local fn="$1"
  local value
  if value="$("${fn}" 2>/dev/null || true)"; then
    value="${value//[!0-9]/}"
    if [[ -n "${value}" ]]; then
      echo "${value}"
      return 0
    fi
  fi
  echo 0
  return 0
}

while true; do
  CPU_PERCENT="$(safe_metric read_cpu_percent)"
  RAM_PERCENT="$(safe_metric read_ram_percent)"
  DISK_PERCENT="$(safe_metric read_disk_percent)"
  NETWORK_KBPS="$(safe_metric read_network_kbps)"
  LOAD_AVERAGE="$(safe_metric read_load_average)"
  PROCESSES_COUNT="$(safe_metric read_process_count)"

  if ! curl -fsS --connect-timeout 5 --max-time 10 -X POST "${CORE_URL}/api/v1/nodes/heartbeat" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${AUTH_TOKEN}" \
    -d "{\"name\":\"${NODE_NAME}\",\"region\":\"${NODE_REGION}\",\"ip_address\":\"${NODE_IP}\",\"tags\":${TAGS_JSON},\"status\":\"${STATUS}\",\"metrics\":{\"cpu_percent\":${CPU_PERCENT},\"ram_percent\":${RAM_PERCENT},\"disk_percent\":${DISK_PERCENT},\"network_kbps\":${NETWORK_KBPS},\"load_average\":${LOAD_AVERAGE},\"processes_count\":${PROCESSES_COUNT}}}" \
    >/dev/null; then
    log "heartbeat send failed: core=${CORE_URL} node=${NODE_NAME} ip=${NODE_IP}"
  fi
  sleep "${INTERVAL}"
done
EOF

cat >"${TMP_EXEC_BIN}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="/etc/meza-node/node.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE}" >&2
  exit 1
fi

# shellcheck disable=SC1090
source "${ENV_FILE}"

POLL_INTERVAL="${TERMINAL_POLL_INTERVAL:-2}"

log() {
  echo "[meza-node-exec] $*" >&2
}

while true; do
  HEADERS_FILE="$(mktemp)"
  BODY_FILE="$(mktemp)"
  RESULT_FILE="$(mktemp)"
  cleanup_loop() {
    rm -f "${HEADERS_FILE}" "${BODY_FILE}" "${RESULT_FILE}"
  }

  HTTP_CODE="$(curl -sS --connect-timeout 5 --max-time 20 -X POST "${CORE_URL}/api/v1/terminal/agent/poll" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${AUTH_TOKEN}" \
    -d "{\"node_name\":\"${NODE_NAME}\"}" \
    -D "${HEADERS_FILE}" \
    -o "${BODY_FILE}" \
    -w "%{http_code}" || true)"

  if [[ "${HTTP_CODE}" == "200" ]]; then
    TASK_ID="$(awk -F': ' 'tolower($1)=="x-meza-task-id"{gsub("\r","",$2); print $2}' "${HEADERS_FILE}" | head -n1)"
    COMMAND_TEXT="$(cat "${BODY_FILE}")"

    if [[ -n "${TASK_ID}" && -n "${COMMAND_TEXT}" ]]; then
      EXIT_CODE=0
      OUTPUT_TEXT=""
      if ! OUTPUT_TEXT="$(bash -lc "${COMMAND_TEXT}" 2>&1)"; then
        EXIT_CODE=$?
      fi

      printf '%s' "${OUTPUT_TEXT}" > "${RESULT_FILE}"
      STATUS="completed"
      if (( EXIT_CODE != 0 )); then
        STATUS="failed"
      fi

      if ! curl -sS --connect-timeout 5 --max-time 30 -X POST "${CORE_URL}/api/v1/terminal/agent/result/${TASK_ID}?status=${STATUS}&exit_code=${EXIT_CODE}" \
        -H "Authorization: Bearer ${AUTH_TOKEN}" \
        -H "Content-Type: text/plain; charset=utf-8" \
        --data-binary @"${RESULT_FILE}" >/dev/null; then
        log "failed to post result: task=${TASK_ID} node=${NODE_NAME}"
      fi
    else
      log "poll returned empty task payload (task_id=${TASK_ID})"
    fi
  elif [[ "${HTTP_CODE}" != "204" && -n "${HTTP_CODE}" ]]; then
    log "poll failed: http=${HTTP_CODE} core=${CORE_URL}"
  fi

  cleanup_loop
  sleep "${POLL_INTERVAL}"
done
EOF

run_as_root mkdir -p "$(dirname "${ENV_PATH}")" "${INSTALL_DIR}"
run_as_root install -m 600 "${TMP_ENV}" "${ENV_PATH}"
run_as_root install -m 755 "${TMP_BIN}" "${BIN_PATH}"
run_as_root install -m 755 "${TMP_EXEC_BIN}" "${EXEC_BIN_PATH}"

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

  cat >"${TMP_EXEC_SERVICE}" <<EOF
[Unit]
Description=Meza-Node terminal execution agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${EXEC_BIN_PATH}
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  run_as_root install -m 644 "${TMP_SERVICE}" "/etc/systemd/system/${SERVICE_NAME}.service"
  run_as_root install -m 644 "${TMP_EXEC_SERVICE}" "/etc/systemd/system/${EXEC_SERVICE_NAME}.service"
  run_as_root systemctl daemon-reload
  run_as_root systemctl enable "${SERVICE_NAME}.service"
  run_as_root systemctl enable "${EXEC_SERVICE_NAME}.service"
  run_as_root systemctl restart "${SERVICE_NAME}.service"
  run_as_root systemctl restart "${EXEC_SERVICE_NAME}.service"
  SERVICE_MODE="systemd"
else
  LOG_FILE="${INSTALL_DIR}/heartbeat.log"
  PID_FILE="${INSTALL_DIR}/heartbeat.pid"
  EXEC_LOG_FILE="${INSTALL_DIR}/exec.log"
  EXEC_PID_FILE="${INSTALL_DIR}/exec.pid"
  if run_as_root test -f "${PID_FILE}"; then
    run_as_root sh -c "kill \$(cat '${PID_FILE}') >/dev/null 2>&1 || true"
  fi
  if run_as_root test -f "${EXEC_PID_FILE}"; then
    run_as_root sh -c "kill \$(cat '${EXEC_PID_FILE}') >/dev/null 2>&1 || true"
  fi
  run_as_root sh -c "nohup '${BIN_PATH}' >> '${LOG_FILE}' 2>&1 & echo \$! > '${PID_FILE}'"
  run_as_root sh -c "nohup '${EXEC_BIN_PATH}' >> '${EXEC_LOG_FILE}' 2>&1 & echo \$! > '${EXEC_PID_FILE}'"
  SERVICE_MODE="background"
fi

echo
echo "Node registered and connected automatically."
echo "Agent service mode: ${SERVICE_MODE}"
echo "No extra clicks required. The node now sends heartbeat and executes terminal tasks."
