# Hugo Admin 文档

Hugo Admin 是一个用于管理 Hugo 静态站点的轻量级 Web 管理界面。
本文档介绍如何部署、配置和使用 Hugo Admin，以及面向开发者的内部参考。

## 主要功能

- **📊 仪表盘**：博客统计概览与快捷操作
- **📝 文章管理**：按分类、标签浏览、搜索、筛选文章
- **✏️ Markdown 编辑器**：在线编辑、自动保存、键盘快捷键
- **🖼 剪贴板图片粘贴**：截图后 `Ctrl+V` 直接插入图片
- **🚀 Hugo 服务控制**：启停 Hugo dev server，实时日志
- **🔍 高级搜索**：支持全文搜索与分类、标签过滤
- **⚡ 实时更新**：基于 WebSocket 的实时日志推送
- **💾 缓存系统**：基于 SQLite 的文章索引与缓存

## 技术栈

- **后端**：Flask + Flask-SocketIO
- **前端**：Tailwind CSS + Alpine.js
- **实时通信**：WebSocket (Socket.IO)
- **静态站点生成器**：Hugo Extended

## 文档导览

### 🚀 快速开始

第一次接触 Hugo Admin？从这里开始。

- [快速开始](QUICKSTART.md) — 5 分钟跑起来
- [Docker 部署](docker.md) — 生产环境容器化部署
- [Demo 服务器部署](plan/demo-deployment.md) — 公网 demo 实例方案

### 📖 使用指南

日常使用与功能说明。

- [缓存使用](CACHE_USAGE.md) — 缓存机制与维护
- [GitHub 设置](GITHUB_SETUP.md) — 仓库与发布流程
- [剪贴板图片粘贴](CLIPBOARD_IMAGE_PASTE.md) — 编辑器图片粘贴功能

### 🛠 开发文档

面向贡献者与开发者的内部文档。

- [Frontmatter 重构](FRONTMATTER_REFACTOR.md) — frontmatter 处理的演进
- [整体发布功能](development/system-publish.md) — 一键发布整站
- [测试报告](TEST_REPORT.md) — 测试结果与覆盖

### 📋 参考文档

历史变更与排障记录。

- [修复列表](FIXES.md) — Bug 修复清单
- [修复摘要](FIX_SUMMARY.md) — 修复工作的总结
- [变更日志](CHANGELOG.md) — 版本变更记录
- [Claude 集成](CLAUDE.md) — Claude 协作说明

### 🤝 贡献与安全

- [贡献指南](CONTRIBUTING.md) — 如何参与贡献
- [行为准则](CODE_OF_CONDUCT.md) — 社区公约
- [安全政策](SECURITY.md) — 漏洞报告

## 获取帮助

- 在 GitHub 提交 [Issue](https://github.com/Svtter/hugo-admin/issues)
- 查看 [GitHub Discussions](https://github.com/Svtter/hugo-admin/discussions)
- 查阅 [README](https://github.com/Svtter/hugo-admin/blob/main/README.md) 与 [中文 README](https://github.com/Svtter/hugo-admin/blob/main/README.zh-CN.md)
