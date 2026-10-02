# 候选版测试与验证范围

公开源码由稳定版本的 tracked 源码导出，单独初始化 Git。生产项目与私有备份仓库继续独立维护。

## 安全单元测试

使用 Go 1.24.0 或以上兼容版本：

```bash
go test -count=1 -timeout 120s ./...
go build -o bin/xiaohongshu-mcp-chatgpt .
go build -o bin/xiaohongshu-login ./cmd/login
```

测试包括分享链接校验/重定向边界、初始评论数量限制、图片响应格式与 widget/模型入口分离、附件句柄有效期、图片路径校验、发布正文规范化、AI 声明状态机和超时预算。

默认测试用的 Cookie/Token 字符串、1×1 图片 base64、账号/笔记标识均为模拟数据，不是生产凭据或真实账号样本。临时文件由 `t.TempDir()` 隔离和回收。

不要为常规 CI 使用 `-tags integration`。标记为 integration 的测试是人工验证入口，可能触网、启动浏览器或依赖账号。真实发布测试另有跳过保护，本次包装不运行任何真实写操作。

## 本公开包装的代码差异

- 功能实现从稳定源码保留，Go module 名称保持上游路径，以免重写 imports。
- `feed_image_test.go` 分别验证无 host 的 `file:` URL 和具有 host 的 `ftp:` URL 被拒绝，匹配现有验证顺序。
- `cookies/cookies_test.go` 同时设置 `TMPDIR`、`TMP` 和 `TEMP`，让临时目录隔离适用于 Linux/macOS/Windows。
- 补充 Apache-2.0 修改标记、独立配置与文档；没有调整生产功能或浏览器/session 策略。
- 视频参数描述改为“服务端本机可读路径”，去掉上游通用用户目录示例；参数类型与发布行为不变。

## 审计边界

扫描覆盖候选目录中的源码、测试、配置模板、文档、Docker/Compose 和 GitHub Actions，检查凭据模式、个人绝对路径及高熵字符串；依赖校验和、合成测试图片等已知非凭据需人工分类。

基于文本和熵的扫描不能数学证明不存在秘密，也不能证明程序没有漏洞。构建/单元测试不等于 Docker 完整镜像构建、网页端实际交互或 ChatGPT 官方客户端视觉验收。未导入真实 Cookie 的候选环境不会进行账号验证。
