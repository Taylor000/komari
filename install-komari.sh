#!/usr/bin/env bash

set -Eeuo pipefail

VERSION="1.2.0"
REPOSITORY="Taylor000/komari"
INSTALL_DIR="/opt/komari"
BINARY_PATH="${INSTALL_DIR}/komari"
SERVICE_NAME="komari"
PORT="25774"

usage() {
    cat <<'EOF'
Komari 1.2.0 自用版安装脚本

用法：
  install-komari.sh [--port 端口]

选项：
  --port PORT   监听端口，默认 25774
  -h, --help    显示帮助
EOF
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --port)
            [ "$#" -ge 2 ] || { echo "错误：--port 缺少端口号" >&2; exit 1; }
            PORT="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "错误：未知参数 $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

case "$PORT" in
    ''|*[!0-9]*) echo "错误：端口必须是 1-65535 的整数" >&2; exit 1 ;;
esac
if [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then
    echo "错误：端口必须是 1-65535 的整数" >&2
    exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
    echo "错误：请使用 root 权限运行（例如通过 sudo）。" >&2
    exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
    echo "错误：当前系统没有 systemd，无法创建 Komari 服务。" >&2
    exit 1
fi

case "$(uname -m)" in
    x86_64|amd64)
        ARCH="amd64"
        EXPECTED_SHA256="085537b89f5d42b39d906d24cec13ad21736667ccd48feedecd3cafa339b4e2d"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        EXPECTED_SHA256="60505e5576b7ec8c7d5790d5747684f25908c09585be89e6218e78c80bf19f11"
        ;;
    i386|i486|i586|i686)
        ARCH="386"
        EXPECTED_SHA256="fb78bec99c8311ed876a6f805c62902d69e12b92e000096e35cd43bda97c5223"
        ;;
    riscv64)
        ARCH="riscv64"
        EXPECTED_SHA256="79025fc8e17aa9a7714d41ab3ed7cd95a5ad8edf35547b6095d4a819f79613c4"
        ;;
    *)
        echo "错误：不支持的系统架构 $(uname -m)" >&2
        exit 1
        ;;
esac

if ! command -v sha256sum >/dev/null 2>&1; then
    echo "错误：系统缺少 sha256sum。" >&2
    exit 1
fi

FILE_NAME="komari-linux-${ARCH}"
DOWNLOAD_URL="https://raw.githubusercontent.com/${REPOSITORY}/main/release/${VERSION}/${FILE_NAME}"
TEMP_FILE="$(mktemp)"
trap 'rm -f "$TEMP_FILE"' EXIT

echo "正在安装 Komari ${VERSION} (${ARCH})..."
if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 15 "$DOWNLOAD_URL" -o "$TEMP_FILE"
elif command -v wget >/dev/null 2>&1; then
    wget -O "$TEMP_FILE" "$DOWNLOAD_URL"
else
    echo "错误：系统需要 curl 或 wget 才能下载安装文件。" >&2
    exit 1
fi

ACTUAL_SHA256="$(sha256sum "$TEMP_FILE" | awk '{print $1}')"
if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
    echo "错误：二进制 SHA-256 校验失败，已停止安装。" >&2
    echo "期望：$EXPECTED_SHA256" >&2
    echo "实际：$ACTUAL_SHA256" >&2
    exit 1
fi

mkdir -p "$INSTALL_DIR/data"
if [ -f "$BINARY_PATH" ]; then
    BACKUP_PATH="${BINARY_PATH}.backup.$(date +%Y%m%d%H%M%S)"
    cp -p "$BINARY_PATH" "$BACKUP_PATH"
    echo "原二进制已备份到：$BACKUP_PATH"
fi
install -m 0755 "$TEMP_FILE" "$BINARY_PATH"

cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=Komari Monitor 1.2.0
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${BINARY_PATH} server -l 0.0.0.0:${PORT}
Restart=on-failure
RestartSec=5s
User=root

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "${SERVICE_NAME}.service"
systemctl restart "${SERVICE_NAME}.service"

if ! systemctl is-active --quiet "${SERVICE_NAME}.service"; then
    echo "错误：Komari 服务启动失败，请检查日志：" >&2
    systemctl status "${SERVICE_NAME}.service" --no-pager -l >&2 || true
    exit 1
fi

echo
echo "Komari ${VERSION} 安装完成。"
echo "访问地址：http://服务器IP:${PORT}"
echo "数据目录：${INSTALL_DIR}/data"
echo "查看日志：journalctl -u ${SERVICE_NAME} -n 100 --no-pager"
echo
journalctl -u "${SERVICE_NAME}" -n 30 --no-pager || true
