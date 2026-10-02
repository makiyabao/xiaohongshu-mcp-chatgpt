# xiaohongshu-mcp-chatgpt

本项目基于 [xpzouying/xiaohongshu-mcp](https://github.com/xpzouying/xiaohongshu-mcp) 修改，主要增加面向 ChatGPT / OpenAI Secure MCP Tunnel 的集成与功能扩展。

让 ChatGPT 通过 OpenAI Secure MCP Tunnel 读取和操作小红书。

这是独立维护的衍生版本，非小红书官方项目、非 OpenAI 官方项目。使用者应自行遵守相关平台规则；不建议高频自动化写操作。保留上游 Apache-2.0 许可证与贡献者署名，修改说明见 [NOTICE](NOTICE)。

## 主要新增能力

源码保留以下工具及扩展；原有搜索、详情和互动能力来自上游或在其基础上扩展。

| 能力 | 对应工具 / 行为 |
| --- | --- |
| ChatGPT 官方客户端接入 | 标准 HTTP MCP，配合独立运行的官方 `tunnel-client` |
| 首页、搜索、笔记详情和个人主页 | `list_feeds`、`search_feeds`、`get_feed_detail`、`get_my_profile` |
| 分享链接解析 | `resolve_xhs_link`，支持短链、完整笔记 URL 和分享文案 |
| 一次调用读取分享笔记 | `read_feed_by_url`，同页读取正文、作者、图像索引、视频基本元数据及互动数据 |
| 初始评论 | 聚合读取保留页面初始的最多 10 条一级评论；不足 10 条时如实返回，不默认滚动全量加载或展开大量回复 |
| 模型读图 | `get_feed_image_for_model` 返回单张标准 MCP ImageContent，不附带图片 widget |
| 给用户展示图片 | `get_feed_image` 返回图片与 MCP Apps 查看器，保留 `ui/initialize` handshake |
| 点赞、收藏 | `like_feed`、`favorite_feed` |
| 评论、回复 | `post_comment_to_feed`、`reply_comment_in_feed` |
| 关注、取关 | `follow_user`、`unfollow_user`；先读状态、幂等、单次一个账号，支持受保护用户 |
| ChatGPT 图片附件导入 | `import_chatgpt_images`，下载附件到服务端，返回 2 小时有效的 `image_tokens` |
| 图文发布 | `publish_content`，兼容图片 URL、服务端本地图片和暂存句柄 |
| 官方 AI 合成声明 | `ai_generated=true` 时选择并复核“笔记含AI合成内容”；不能确认时中止发布 |
| 发布保护 | 本地图片存在/可读/普通文件校验、阶段日志、正文读回校验、5 分钟总预算和每图最多 60 秒上传等待 |

模型读图与用户展示是两个独立入口。`image_index` 从 0 开始，多张滑图逐张调用，不一次返回整篇图片二进制。聚合链接读取不下载完整视频，也不自动分析视频内容。

图片展示/模型读图、分享读取、附件图文发布及 AI 声明在维护者的原部署中有验收记录。本公开候选版只运行无真实账号的单元测试，不承诺所有客户端、账号或网页版本行为一致。静默读图的呈现由宿主客户端决定；工具本身不挂 widget。查看 [测试说明](docs/TESTING.md) 了解验证范围。

## 架构

`ChatGPT → OpenAI Secure MCP Tunnel → Xiaohongshu MCP → Browser / Xiaohongshu`

MCP 位于私有运行环境中；`tunnel-client` 在能访问 MCP 的机器上发起出站 HTTPS 连接。Tunnel 的身份与 runtime API key 由使用者独立配置，本项目不内置任何凭据，也不替使用者管理账号。公开源码仓库并不等于公开共享一个 MCP 服务或在 ChatGPT 商店发布 App。

## 安装要求

- 推荐 Linux x86_64 VPS 或独立 Linux 开发环境。
- Docker Engine 与 Docker Compose v2（推荐）；源码开发需要 Go 1.24.0 或以上兼容版本。
- 当前 Dockerfile 构建 `linux/amd64`，镜像内包含浏览器运行库和中文字体；构建时从上游浏览器 CDN 下载并校验浏览器。ARM 环境需自行调整并验证。
- 若直接运行 Go 程序，需自行提供浏览器的系统依赖；程序使用上游内置浏览器下载机制，浏览器版本来自 `browser/browser_version.txt`。
- 官方 OpenAI `tunnel-client`、具备相应 Tunnel 权限的 OpenAI 组织，以及可使用开发者模式的目标 ChatGPT 工作区。
- 使用者自己登录自己的小红书账号；首次扫码、风险验证和重新授权需人工完成。

源码保留上游 Go module 路径以兼容现有 imports，这不代表本项目由上游官方发布。

## 快速开始

### 1. 获取源码与配置

下载本项目源码到自己的目录（本候选阶段尚未发布 GitHub 仓库）。以下命令在项目根目录执行：

```bash
cp .env.example .env
chmod 600 .env
mkdir -p data images
```

编辑 `.env`，只配置自己的可选设置。模板没有真实凭据；Tunnel key 不放入这个文件。不要把 `.env` 或运行目录提交到 Git。

### 2. 构建并启动自己的 MCP

```bash
docker compose build
docker compose up -d
docker compose ps
curl --fail http://127.0.0.1:18060/health
```

使用项目源码构建本衍生版本；不要改用上游预构建镜像，否则新增工具可能不存在。公开模板仅映射 `127.0.0.1:18060:18060`，无需开放公网入站端口。镜像中的服务监听容器内部端口，宿主机绑定仍是 loopback。

登录态保存在 `data` 卷。图片及附件暂存由现有 `os.TempDir()` 逻辑生成，Compose 将 `TMPDIR` 指向 `images` 卷；短期句柄存在进程内存中，重启后需重新导入附件。到期文件按既有清理逻辑回收，成功发布后导入句柄对应文件会清理。

直接在 Linux 宿主运行源码构建的程序时，必须显式设置 loopback 地址；现有二进制的 `-port` 默认值是 `:18060`。`.env` 由 Compose 读取，Go 程序本身不自动加载该文件：

```bash
go build -o bin/xiaohongshu-mcp-chatgpt .
COOKIES_PATH="$PWD/data/cookies.json" TMPDIR="$PWD/images" \
  ./bin/xiaohongshu-mcp-chatgpt -port 127.0.0.1:18060
```

### 3. 配置官方 Tunnel

从 [OpenAI Secure MCP Tunnel 官方文档](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels) 获取 `tunnel-client` 与自己的 Tunnel 身份。具体账户授权和安装步骤以官方文档为准。

在 MCP 所在的运行环境中执行；密钥用交互输入，避免落入 shell history：

```bash
tunnel-client help quickstart
read -r -s -p 'Runtime API key: ' CONTROL_PLANE_API_KEY
printf '\n'
export CONTROL_PLANE_API_KEY
tunnel-client init --profile xhs-local \
  --tunnel-id YOUR_TUNNEL_ID_HERE \
  --mcp-server-url http://127.0.0.1:18060/mcp
tunnel-client doctor --profile xhs-local --explain
tunnel-client run --profile xhs-local
```

这些是本机 HTTP MCP 的配置步骤；凭据只属于你的运行环境。若设置了 MCP 侧 `AUTH_TOKEN`，还需按官方客户端说明配置上游鉴权，请勿把它混同于 Tunnel runtime key。使用客户端自身的 health/ready 检查，确认其持续运行后再连接 ChatGPT。

### 4. 连接 ChatGPT 并登录小红书

在具有开发者模式权限的 ChatGPT 中创建自定义 App：进入 Apps/Plugins 的开发者连接入口，在 Connection 中选择 **Tunnel**，选择自己关联到该工作区的 Tunnel 或填写自己的 `tunnel_id`。入口名称、权限和可用性以当前官方客户端为准。

通过该 App 调用 `get_login_qrcode`，用自己的手机小红书完成扫码，再调用 `check_login_status`。需要验证码或风险验证时停止并由本人处理，不要自动重试。二维码也是短期敏感运行数据，不应保存到源码仓库。

### 5. 日常读取与按需发布

把分享链接交给 ChatGPT，让它调用 `read_feed_by_url`。需要理解图片时调用 `get_feed_image_for_model`；你明确要求展示图片时，调用 `get_feed_image`。旧的 `resolve_xhs_link` 与 `get_feed_detail` 入口继续保留。

当前聊天附件的发布流程是：

1. 客户端提供附件对象，调用 `import_chatgpt_images`（schema 使用 `openai/fileParams`）。
2. 把返回的 `image_tokens` 交给 `publish_content`，并传 `images: []`。
3. AI 图片需显式传 `ai_generated: true`。先用 `visibility: "仅自己可见"` 自测，并由用户确认后再使用公开可见发布。

ChatGPT 的 `/mnt/data/...` 与远端服务不共享文件系统。附件导入需要宿主提供可下载的 HTTPS 文件对象；不能用一条本机路径替代。其他客户端若不支持该附件能力，可使用自己受控的图片 HTTPS URL 或服务端已有的可读图片路径。

## 安全说明

- 不要提交 Cookie、Token、API key、Tunnel runtime key、真实账号信息、二维码、日志、临时图片或浏览器 profile；不要公开 `runtime.env`。
- 登录态仅保存在自己的本地/服务器运行目录。限制这些目录的访问权限；维护者无法替你恢复、刷新或保护账号。
- MCP 不提供完整多租户授权隔离。按个人可信客户端部署，并保持私有 listener。代码兼容服务端本地路径上传，因此连接权限必须只授予可信使用者。
- 分享链接解析限制小红书域名、重定向次数和访问时间；附件导入限制 HTTPS、公网目的地址、真实图片类型及每张 15MB。保护范围详见代码，不应当作通用下载代理。
- 写操作可能触发平台风控。建议以读取为主，谨慎使用发布、点赞、收藏、评论、回复、关注等功能；禁止短时间批量关注/取消关注，不自动重试不确定结果。
- 遇到扫码、验证码、访问异常或频率限制时人工处理；不得用于绕过平台限制。
- 更新工具 schema 或 UI resource 后，在 ChatGPT 刷新/重新连接 App；旧对话可能需要新开对话才能使用更新后的工具列表。

## 开发与测试

```bash
go test -count=1 -timeout 120s ./...
go build -o bin/xiaohongshu-mcp-chatgpt .
go build -o bin/xiaohongshu-login ./cmd/login
```

默认测试使用临时目录、伪造的最小数据和本地测试 HTTP server，不需要真实 Cookie。**不要为公共 CI 启用 `-tags integration`**：这些测试可能启动浏览器、触网或依赖登录态。公开候选测试记录和包装差异见 [docs/TESTING.md](docs/TESTING.md)，本次准备结果见 [公开检查记录](docs/PUBLICATION_CHECK.md)。

## 上游项目与许可证

感谢 [xpzouying/xiaohongshu-mcp](https://github.com/xpzouying/xiaohongshu-mcp) 及原贡献者。本仓保留原始 [LICENSE](LICENSE)（Apache License 2.0）和 `.all-contributorsrc`；新增与修改说明位于 [NOTICE](NOTICE)。修改过的文件保留醒目的修改标记，未改写 Apache-2.0 正文。

本项目名称仅描述兼容目标，不表示与小红书、OpenAI 或上游维护者存在官方合作、背书或授权关系。
