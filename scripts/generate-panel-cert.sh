#!/usr/bin/env bash
set -euo pipefail

TLS_HOST="${1:-${MEZA_PANEL_TLS_HOST:-localhost}}"
OUT_DIR="${2:-./deploy/certs}"
CRT_PATH="${OUT_DIR}/panel.crt"
KEY_PATH="${OUT_DIR}/panel.key"

mkdir -p "${OUT_DIR}"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required to generate panel certificate" >&2
  exit 1
fi

SAN_DNS="DNS:localhost,DNS:meza-panel-ssl,DNS:meza-panel"
if [[ "${TLS_HOST}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  SAN="IP:${TLS_HOST},IP:127.0.0.1,${SAN_DNS}"
  CN="${TLS_HOST}"
else
  SAN="DNS:${TLS_HOST},IP:127.0.0.1,${SAN_DNS}"
  CN="${TLS_HOST}"
fi

TMP_CONF="$(mktemp)"
cleanup() {
  rm -f "${TMP_CONF}"
}
trap cleanup EXIT

cat >"${TMP_CONF}" <<EOF
[req]
default_bits = 4096
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = ${CN}

[v3_req]
subjectAltName = ${SAN}
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
EOF

openssl req -x509 -nodes -newkey rsa:4096 \
  -days 3650 \
  -keyout "${KEY_PATH}" \
  -out "${CRT_PATH}" \
  -config "${TMP_CONF}"

chmod 600 "${KEY_PATH}"
chmod 644 "${CRT_PATH}"

echo "panel certificate generated:"
echo "  cert: ${CRT_PATH}"
echo "  key : ${KEY_PATH}"
echo "  SAN : ${SAN}"
