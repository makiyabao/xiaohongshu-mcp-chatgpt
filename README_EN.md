# xiaohongshu-mcp-chatgpt

This independent derivative is based on [xpzouying/xiaohongshu-mcp](https://github.com/xpzouying/xiaohongshu-mcp). It adds ChatGPT integration through OpenAI Secure MCP Tunnel and extends the existing MCP tools.

Read and operate Xiaohongshu from ChatGPT through a private MCP server. See the [Chinese README](README.md) for setup, the tool list, and operating boundaries.

Highlights include share-link resolution, single-call note reading with up to ten initially available top-level comments, separate model-readable and user-visible image tools, ChatGPT attachment import, expiring image handles, and verified selection of the official AI-content declaration during publishing.

The derivative preserves the upstream Go module path and Apache-2.0 [LICENSE](LICENSE), along with contributor attributions. [NOTICE](NOTICE) documents derivative changes. This is not an official Xiaohongshu or OpenAI project, nor an official upstream release.

Use your own account and credentials, obey platform rules, and favor reads over automated writes. Keep the server private and protect login data. Tool schema availability and image presentation depend on the host client. Source publication is separate from operating or submitting a public ChatGPT app.

The candidate uses a loopback-only Compose port mapping. For direct execution, explicitly pass `-port 127.0.0.1:18060`; the unchanged binary default is `:18060`. Default unit tests do not require a real account. Do not enable integration tests in public CI.

See [test scope](docs/TESTING.md) and the [candidate preparation checks](docs/PUBLICATION_CHECK.md).
