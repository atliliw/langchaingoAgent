# Lanchaingo Agent 部署文档

## 服务器信息

| 项目 | Rust 版本 | Go 版本 (本服务) |
|------|-----------|-----------------|
| 后端端口 | 8090 | **8091** |
| Nginx 端口 | 8081 | **8082** |
| 前端访问 | http://192.168.10.100:8081 | **http://192.168.10.100:8082** |
| 工作目录 | `/opt/langchainrust/demo/` | **`/opt/lanchaingo-agent/`** |

> 注意：Go 版本与 Rust 版本并行运行，使用不同端口和目录，互不冲突。

## 架构

```
Nginx (192.168.10.100:8082)  →  前端静态文件 (Go 版本)
                         ↘  反向代理 /api/* → 后端 :8091

后端 (Go) → :8091
├── SQLite      : conversations.db    对话历史
├── Qdrant      : http://192.168.10.100:6334  向量库
└── MongoDB     : mongodb://admin:admin123@192.168.10.100:27017  BM25
```

## 配置

配置文件 `/opt/lanchaingo-agent/config.toml`：

```toml
[server]
host = "0.0.0.0"
port = 8091                      # 后端监听端口
upload_dir = "uploads"

[openai]
api_key = "sk-a827c856f53e459bbfad5b8e1b962fc7"
base_url = "https://dashscope.aliyuncs.com/compatible-mode/v1"
chat_model = "qwen-plus"
embedding_model = "text-embedding-v1"

[qdrant]
url = "http://192.168.10.100:6334"
collection_name = "demo_documents"
vector_size = 1536

[mongodb]
uri = "mongodb://admin:admin123@192.168.10.100:27017"
database = "langchainrust_demo"

[sqlite]
db_path = "/opt/lanchaingo-agent/conversations.db"
```

## 一键部署（本地 Windows → 服务器）

```powershell
# 1. 交叉编译 Linux 二进制
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o lanchaingo-agent .

# 2. 创建服务器目录
ssh -i ~\.ssh\demo_deploy_key root@192.168.10.100 "mkdir -p /opt/lanchaingo-agent/frontend /opt/lanchaingo-agent/uploads"

# 3. 上传文件
scp -i ~\.ssh\demo_deploy_key lanchaingo-agent root@192.168.10.100:/opt/lanchaingo-agent/
scp -i ~\.ssh\demo_deploy_key deploy\config.toml root@192.168.10.100:/opt/lanchaingo-agent/
scp -i ~\.ssh\demo_deploy_key frontend\index.html root@192.168.10.100:/opt/lanchaingo-agent/frontend/
scp -i ~\.ssh\demo_deploy_key -r frontend\css root@192.168.10.100:/opt/lanchaingo-agent/frontend/
scp -i ~\.ssh\demo_deploy_key -r frontend\js root@192.168.10.100:/opt/lanchaingo-agent/frontend/

# 4. 服务器端配置
ssh -i ~\.ssh\demo_deploy_key root@192.168.10.100
chmod +x /opt/lanchaingo-agent/lanchaingo-agent
chown -R root:root /opt/lanchaingo-agent
```

## Systemd 服务

文件 `/etc/systemd/system/lanchaingo-agent.service`：

```ini
[Unit]
Description=Lanchaingo Agent Backend (Go Port)
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/lanchaingo-agent
ExecStart=/opt/lanchaingo-agent/lanchaingo-agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable lanchaingo-agent
systemctl start lanchaingo-agent
```

## Nginx 配置

Go 版本的 Nginx server block 已添加在 `/etc/nginx/nginx.conf` 中（紧接 Rust 版本之后）：

```nginx
server {
    listen 8082;
    server_name _;

    root /opt/lanchaingo-agent/frontend;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8091;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }

    client_max_body_size 50M;
}
```

## 关键路径

| 资源 | 路径 |
|------|------|
| 后端程序 | `/opt/lanchaingo-agent/lanchaingo-agent` |
| 配置文件 | `/opt/lanchaingo-agent/config.toml` |
| 前端文件 | `/opt/lanchaingo-agent/frontend/` |
| SQLite 数据库 | `/opt/lanchaingo-agent/conversations.db` |
| 上传目录 | `/opt/lanchaingo-agent/uploads/` |
| Systemd 服务 | `/etc/systemd/system/lanchaingo-agent.service` |
| Nginx 配置 | `/etc/nginx/nginx.conf` (第二个 server block) |

## 常用命令

```bash
# 查看服务状态
systemctl status lanchaingo-agent

# 查看日志
journalctl -u lanchaingo-agent -f

# 重启
systemctl restart lanchaingo-agent

# 查看 Nginx 状态
systemctl status nginx
nginx -t && systemctl reload nginx

# 测试 API
curl http://127.0.0.1:8091/api/stats
curl http://127.0.0.1:8082/api/chat/compress-modes   # 通过 Nginx

# 前端访问
http://192.168.10.100:8082
```

## 只更新后端（单文件修改后）

```powershell
# 交叉编译
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o lanchaingo-agent .

# 上传 + 重启
scp -i ~\.ssh\demo_deploy_key lanchaingo-agent root@192.168.10.100:/opt/lanchaingo-agent/
ssh -i ~\.ssh\demo_deploy_key root@192.168.10.100 "systemctl restart lanchaingo-agent"
```

## 只更新前端

```powershell
scp -i ~\.ssh\demo_deploy_key frontend\index.html root@192.168.10.100:/opt/lanchaingo-agent/frontend/
scp -i ~\.ssh\demo_deploy_key -r frontend\js root@192.168.10.100:/opt/lanchaingo-agent/frontend/
scp -i ~\.ssh\demo_deploy_key -r frontend\css root@192.168.10.100:/opt/lanchaingo-agent/frontend/
```

## 验证清单

- [x] 后端运行中 `systemctl status lanchaingo-agent` → `active (running)`
- [x] 端口监听 `ss -tlnp | grep 8091` → Go 后端
- [x] 端口监听 `ss -tlnp | grep 8082` → Nginx
- [x] API 响应 `curl http://127.0.0.1:8091/api/stats` → JSON
- [x] 前端访问 `curl http://127.0.0.1:8082/` → HTTP 200
- [x] 反向代理 `curl http://127.0.0.1:8082/api/stats` → JSON
- [x] 开机自启 `systemctl is-enabled lanchaingo-agent` → enabled
