# Komari 1.2.0 自用归档

本仓库固定保留 **Komari 服务端 1.2.0**，内置对应的 Web 前端，不跟随上游更新。

## 一键安装

适用于使用 systemd 的 Linux（Debian、Ubuntu、CentOS、AlmaLinux 等）：

```bash
curl -fsSL https://raw.githubusercontent.com/Taylor000/komari/main/install-komari.sh | sudo bash
```

默认安装到 `/opt/komari`，监听端口为 `25774`。指定端口：

```bash
curl -fsSL https://raw.githubusercontent.com/Taylor000/komari/main/install-komari.sh | sudo bash -s -- --port 3000
```

安装完成后访问 `http://服务器IP:25774`，初始账号密码请查看安装输出或运行：

```bash
sudo journalctl -u komari -n 100 --no-pager
```

## 数据与管理

- 数据目录：`/opt/komari/data`
- 启动：`sudo systemctl start komari`
- 停止：`sudo systemctl stop komari`
- 重启：`sudo systemctl restart komari`
- 日志：`sudo journalctl -u komari -f`

迁移或重装前请完整备份数据目录。安装脚本只安装固定的 `1.2.0`，不会自动升级。

## 说明

- 源码固定在上游标签 `1.2.0`（提交 `567d2b66fb5d05545abd89a2843f1c7c94e8fff4`）。
- 安装脚本和二进制下载均只使用 `Taylor000/komari`。
- 仓库内保留四种 Linux 架构的官方 1.2.0 二进制及 SHA-256，一键脚本不依赖上游 Release。
- 原项目使用 MIT License，版权及许可信息见 [LICENSE](./LICENSE) 与 [NOTICE](./NOTICE)。
