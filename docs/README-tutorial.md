# AI Code 客户端配置教程文档

本目录包含 Claude Code 和 Codex (Cursor/Windsurf) 的安装和配置教程。

## 📁 文件说明

### 独立教程文件
- `tutorial-claude-code.html` - Claude Code 独立教程
- `tutorial-cursor.html` - Cursor IDE 独立教程
- `tutorial-windsurf.html` - Windsurf IDE 独立教程

### 整合版本
- `tutorial-sections.json` - 完整的教程章节 JSON 数据，可直接导入到教程管理系统

## 🚀 如何使用

### 方式 1: 导入到教程管理系统

1. 登录管理后台
2. 进入 **仪表盘设置 → 教程内容管理**
3. 复制 `tutorial-sections.json` 的内容
4. 粘贴到教程设置的输入框中
5. 保存设置

### 方式 2: 单独使用 HTML 文件

每个 HTML 文件都是独立的，可以：
- 直接在浏览器中打开查看
- 嵌入到其他网页中
- 作为单独的帮助文档使用

## 📝 教程内容特点

### 动态变量支持
教程中使用了以下动态变量，会在显示时自动替换：
- `{{BASE_URL}}` - 网站根地址
- `{{CLAUDE_API_URL}}` - Claude API 端点（= BASE_URL）
- `{{OPENAI_API_URL}}` - OpenAI API 端点（= BASE_URL/v1）

### 支持的操作系统
- ✅ Windows
- ✅ macOS
- ✅ Linux / WSL2

### 涵盖的工具
1. **Claude Code** - Anthropic 官方命令行 AI 编码助手
2. **Cursor** - AI 增强的代码编辑器
3. **Windsurf** - Codeium 的 AI IDE

## 📋 教程章节结构

每个教程都包含：
1. 安装步骤（针对不同操作系统）
2. 中转 API 配置方法（推荐方式 + 替代方式）
3. 验证和使用说明

## 🎨 样式说明

所有教程都包含内联 CSS 样式，确保：
- 清晰的视觉层次
- 代码块语法高亮
- 响应式布局
- 提示信息突出显示

## ⚠️ 注意事项

1. **API Key 安全**：教程中使用 `YOUR_API_KEY` 作为占位符，用户需要替换为实际的 API Key
2. **端点差异**：
   - Claude Code 使用基础 URL（不带 /v1）
   - Cursor 和 Windsurf 需要 OpenAI 格式（带 /v1 后缀）
3. **配置方式**：推荐使用全局配置文件或 IDE 内置设置，避免在项目文件中暴露 API Key

## 🔄 更新记录

- 2025-11-23: 初始版本创建，包含三个主要 AI 编码工具的配置教程
