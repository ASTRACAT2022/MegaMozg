#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${MEZA_REPO_URL:-https://github.com/ASTRACAT2022/MegaMozg.git}"
REPO_RAW_BASE="${MEZA_REPO_RAW_BASE:-https://raw.githubusercontent.com/ASTRACAT2022/MegaMozg/main}"
HUB_DIR="${MEZA_HUB_DIR:-/opt/mezamozg-hub}"

print_usage() {
  cat <<'EOF'
MezaMozg unified installer

Usage:
  install.sh -install hub
  install.sh -install node <HUB_IP_OR_HOST> <BOOTSTRAP_TOKEN>
  install.sh -install all

Examples:
  curl -fsSL https://raw.githubusercontent.com/ASTRACAT2022/MegaMozg/main/scripts/install.sh | bash -s -- -install hub
  curl -fsSL https://raw.githubusercontent.com/ASTRACAT2022/MegaMozg/main/scripts/install.sh | bash -s -- -install node 10.0.0.12 prod-bootstrap-token
EOF
}

run_as_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

port_in_use() {
  local port="$1"
  if has_cmd ss && ss -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${port}$"; then
    return 0
  fi
  if has_cmd lsof && lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

pick_core_host_port() {
  if [[ -n "${MEZA_CORE_HOST_PORT:-}" ]]; then
    echo "${MEZA_CORE_HOST_PORT}"
    return
  fi

  if ! port_in_use 8080; then
    echo "8080"
    return
  fi

  for candidate in 18080 28080 38080; do
    if ! port_in_use "${candidate}"; then
      echo "${candidate}"
      return
    fi
  done

  echo "8080"
}

pkg_manager() {
  if has_cmd apt-get; then
    echo "apt"
    return
  fi
  if has_cmd dnf; then
    echo "dnf"
    return
  fi
  if has_cmd yum; then
    echo "yum"
    return
  fi
  echo "unknown"
}

install_pkg_if_missing() {
  local bin="$1"
  local pkg="$2"
  if has_cmd "${bin}"; then
    return
  fi

  local pm
  pm="$(pkg_manager)"
  case "${pm}" in
    apt)
      run_as_root apt-get update -y
      run_as_root apt-get install -y "${pkg}"
      ;;
    dnf)
      run_as_root dnf install -y "${pkg}"
      ;;
    yum)
      run_as_root yum install -y "${pkg}"
      ;;
    *)
      echo "cannot auto-install ${pkg}. install ${bin} manually." >&2
      exit 1
      ;;
  esac
}

ensure_docker() {
  if has_cmd docker; then
    return
  fi
  echo "docker not found, installing..."
  curl -fsSL https://get.docker.com | run_as_root sh
  run_as_root systemctl enable --now docker || true
}

ensure_compose() {
  if docker compose version >/dev/null 2>&1; then
    return
  fi
  local pm
  pm="$(pkg_manager)"
  case "${pm}" in
    apt)
      run_as_root apt-get update -y
      run_as_root apt-get install -y docker-compose-plugin || true
      ;;
    dnf)
      run_as_root dnf install -y docker-compose-plugin || true
      ;;
    yum)
      run_as_root yum install -y docker-compose-plugin || true
      ;;
  esac
  if ! docker compose version >/dev/null 2>&1; then
    echo "docker compose plugin is missing. install it manually and rerun installer." >&2
    exit 1
  fi
}

generate_token() {
  if has_cmd openssl; then
    openssl rand -hex 24
    return
  fi
  if has_cmd sha256sum; then
    date +%s%N | sha256sum | awk '{print $1}' | cut -c1-48
    return
  fi
  if has_cmd shasum; then
    date +%s%N | shasum -a 256 | awk '{print $1}' | cut -c1-48
    return
  fi
  date +%s%N
}

set_env_value() {
  local file="$1"
  local key="$2"
  local value="$3"
  if grep -q "^${key}=" "${file}"; then
    sed -i.bak "s#^${key}=.*#${key}=${value}#g" "${file}"
  else
    printf '\n%s=%s\n' "${key}" "${value}" >>"${file}"
  fi
}

set_env_value_root() {
  local file="$1"
  local key="$2"
  local value="$3"
  run_as_root sh -c '
file="$1"
key="$2"
value="$3"
if grep -q "^${key}=" "${file}"; then
  sed -i.bak "s#^${key}=.*#${key}=${value}#g" "${file}"
else
  printf "\n%s=%s\n" "${key}" "${value}" >>"${file}"
fi
' _ "${file}" "${key}" "${value}"
}

install_hub() {
  echo "[meza] installing hub stack (core + panel + ssl proxy)"
  install_pkg_if_missing git git
  install_pkg_if_missing curl curl
  install_pkg_if_missing openssl openssl
  ensure_docker
  ensure_compose

  run_as_root mkdir -p "${HUB_DIR}"
  if [[ ! -d "${HUB_DIR}/.git" ]]; then
    run_as_root git clone "${REPO_URL}" "${HUB_DIR}"
  else
    run_as_root git -C "${HUB_DIR}" fetch --all --tags
    run_as_root git -C "${HUB_DIR}" reset --hard origin/main
  fi

  if [[ ! -f "${HUB_DIR}/.env" ]]; then
    run_as_root cp "${HUB_DIR}/.env.example" "${HUB_DIR}/.env"
  fi

  local operator bootstrap node panel_auth_user panel_auth_password panel_auth_session
  operator="$(generate_token)"
  bootstrap="$(generate_token)"
  node="$(generate_token)"
  panel_auth_user="${MEZA_PANEL_BASIC_AUTH_USER:-admin}"
  panel_auth_password="${MEZA_PANEL_BASIC_AUTH_PASSWORD:-$(generate_token | cut -c1-20)}"
  panel_auth_session="$(generate_token)"
  local core_host_port
  core_host_port="$(pick_core_host_port)"
  local host_ip
  host_ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  if [[ -z "${host_ip}" ]]; then
    host_ip="127.0.0.1"
  fi

  set_env_value_root "${HUB_DIR}/.env" MEZA_OPERATOR_TOKEN "${operator}"
  set_env_value_root "${HUB_DIR}/.env" MEZA_PANEL_OPERATOR_TOKEN "${operator}"
  set_env_value_root "${HUB_DIR}/.env" MEZA_BOOTSTRAP_TOKEN "${bootstrap}"
  set_env_value_root "${HUB_DIR}/.env" MEZA_NODE_TOKEN "${node}"
  set_env_value_root "${HUB_DIR}/.env" MEZA_CORE_HOST_PORT "${core_host_port}"
  set_env_value_root "${HUB_DIR}/.env" MEZA_PANEL_TLS_HOST "${host_ip}"
  set_env_value_root "${HUB_DIR}/.env" MEZA_CORE_BASE_URL "http://meza-core:8080"
  set_env_value_root "${HUB_DIR}/.env" MEZA_ALLOW_ANONYMOUS_UI "false"
  set_env_value_root "${HUB_DIR}/.env" MEZA_TERMINAL_SSH_ENABLED "true"
  set_env_value_root "${HUB_DIR}/.env" MEZA_TERMINAL_SSH_USER "root"
  set_env_value_root "${HUB_DIR}/.env" MEZA_TERMINAL_SSH_PORT "22"
  set_env_value_root "${HUB_DIR}/.env" MEZA_TERMINAL_SSH_KEY_PATH "/data/ssh/id_ed25519"
  set_env_value_root "${HUB_DIR}/.env" MEZA_TERMINAL_SSH_STRICT_HOST_KEY_CHECKING "accept-new"
  set_env_value_root "${HUB_DIR}/.env" MEZA_TERMINAL_SSH_KNOWN_HOSTS_PATH "/data/ssh/known_hosts"
  set_env_value_root "${HUB_DIR}/.env" MEZA_PANEL_BASIC_AUTH_ENABLED "true"
  set_env_value_root "${HUB_DIR}/.env" MEZA_PANEL_BASIC_AUTH_USER "${panel_auth_user}"
  set_env_value_root "${HUB_DIR}/.env" MEZA_PANEL_BASIC_AUTH_PASSWORD "${panel_auth_password}"
  set_env_value_root "${HUB_DIR}/.env" MEZA_PANEL_BASIC_AUTH_SESSION_TOKEN "${panel_auth_session}"

  run_as_root mkdir -p "${HUB_DIR}/deploy/certs"
  run_as_root bash "${HUB_DIR}/scripts/generate-panel-cert.sh" "${host_ip}" "${HUB_DIR}/deploy/certs"

  run_as_root docker compose -f "${HUB_DIR}/docker-compose.yml" --env-file "${HUB_DIR}/.env" up -d --build

  cat <<EOF

[meza] hub installation completed
Hub directory: ${HUB_DIR}
Panel URL: https://${host_ip}:1499
Core URL:  http://${host_ip}:${core_host_port}
Panel login: ${panel_auth_user}
Panel password: ${panel_auth_password}
Bootstrap token (save it): ${bootstrap}

Node install command:
curl -fsSL ${REPO_RAW_BASE}/scripts/install.sh | bash -s -- -install node http://${host_ip}:${core_host_port} ${bootstrap}
EOF
}

normalize_core_url() {
  local hub="$1"
  if [[ "${hub}" == http://* || "${hub}" == https://* ]]; then
    echo "${hub}"
  else
    echo "http://${hub}:8080"
  fi
}

install_node() {
  local hub_host="$1"
  local bootstrap_token="$2"
  local core_url
  core_url="$(normalize_core_url "${hub_host}")"

  echo "[meza] installing node and auto-connecting to ${core_url}"
  curl -fsSL "${REPO_RAW_BASE}/scripts/install-node.sh" | bash -s -- --token "${bootstrap_token}" --core-url "${core_url}"
}

if [[ $# -lt 2 ]]; then
  print_usage
  exit 1
fi

if [[ "${1}" != "-install" ]]; then
  print_usage
  exit 1
fi

MODE="${2}"
case "${MODE}" in
  hub)
    install_hub
    ;;
  node)
    if [[ $# -lt 4 ]]; then
      echo "node mode requires: <HUB_IP_OR_HOST> <BOOTSTRAP_TOKEN>" >&2
      print_usage
      exit 1
    fi
    install_node "${3}" "${4}"
    ;;
  all)
    install_hub
    ;;
  *)
    echo "unknown mode: ${MODE}" >&2
    print_usage
    exit 1
    ;;
esac
