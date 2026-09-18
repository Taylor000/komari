# Komari 1.2.0

## 安装

```bash
curl -fsSL https://raw.githubusercontent.com/Taylor000/komari/main/install-komari.sh | sudo bash
```

指定端口：

```bash
curl -fsSL https://raw.githubusercontent.com/Taylor000/komari/main/install-komari.sh | sudo bash -s -- --port 3000
```

也可运行 `tool`，选择 `15. Komari 1.2.0`。

## 管理

```bash
systemctl status komari
systemctl restart komari
journalctl -fu komari
```

数据目录：`/opt/komari/data`
