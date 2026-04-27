# TypeLens

中文文档。英文主文档见 [README.md](./README.md)。

TypeLens 是一个面向 Typeless 的 macOS 桌面端和命令行工具，目标很明确：

- 更高效地管理 Typeless 远端词典
- 更方便地查询 Typeless 历史转写并快速复制文本

桌面端基于 Wails + React + TypeScript，CLI 基于 Cobra。

## 当前功能总览

### 1. 词典管理

- 列出远端 Typeless 词典全部词条
- 新增、编辑、删除词条
- 从文本文件导入词条
  - 文件格式要求：每行一个词
- 使用内置默认词表或自定义文件做差量重置
- 导出当前词典到 `.txt` 文件
- 在桌面端同时展示：
  - 已经同步到远端的词
  - 本地待同步的自动导入词

### 2. 自动导入

- 扫描 Codex、Claude，以及用户手动添加的“其他目录”
- 解析 `history.jsonl` 和相关 `*.jsonl`
- 生成候选词并提供预览
- 支持勾选/取消勾选候选词
- 确认后先写入本地待同步状态，再后台异步同步到远端词典
- 桌面端实时展示扫描和同步日志，包括：
  - 预计要扫描多少文件
  - 当前已扫描多少文件
  - 当前累计解析出多少文本/消息
  - 总共识别出多少原始词
  - 最终候选词数量

### 3. 历史记录查询

- 查询 Typeless 最近转写历史
- 支持关键字过滤
- 支持正则过滤
- 支持上下文模式：
  - `all`
  - `frontmost`
  - `latest`
- 支持最新优先 / 最早优先排序
- 支持一键复制某条历史内容

### 4. 本地优先缓存体验

- 程序启动时优先展示本地缓存
- 后台静默刷新，不打断页面
- 缓存写入文件系统，不依赖浏览器 `localStorage`

## 桌面端交互

- 左侧包含两个主视图：
  - `词典`
  - `历史记录`
- 导入对话框包含两个 Tab：
  - `导入文件`
  - `自动导入`
- 词典词条支持右键刷新
- 快捷键：
  - `Cmd/Ctrl + F`：跳到历史搜索
  - `Cmd + W`：隐藏窗口

## CLI 功能

可用命令包括：

- `typelens dict list`
- `typelens dict add <term>`
- `typelens dict import <file> [--dry-run] [--concurrency N]`
- `typelens dict delete --id <id>`
- `typelens dict clear --yes [--concurrency N]`
- `typelens dict reset --yes [--file <path>] [--concurrency N]`
- `typelens history [--limit N] [--keyword text] [--regex expr] [--context frontmost|latest|all] [--no-copy] [--full]`
- `typelens auto-import`

## 运行前提

当前项目默认面向 macOS 上已经安装并登录 Typeless 的环境。

它会读取 Typeless 本地数据：

- `~/Library/Application Support/Typeless/user-data.json`
- `~/Library/Application Support/Typeless/typeless.db`

TypeLens 自己还会使用这些本地文件：

- 缓存文件：`~/.typelens/cache.json`
- 自动导入待同步状态：`~/Library/Application Support/TypeLens/auto-import-pending.json`
- 默认导出目录：`~/Downloads`

## 项目结构

- `app.go`：Wails 桌面绑定层
- `internal/service/`：服务层、缓存存储、自动导入编排
- `internal/cli/`：命令行入口和子命令
- `pkg/typeless/`：Typeless API、历史、导入导出、认证、自动导入基础能力
- `frontend/src/`：桌面 UI

## 开发方式

### 依赖

- Go `1.26+`
- Node.js / npm
- Wails CLI
- 如果要使用真实 Typeless 数据，需要 macOS 和 Typeless 本地安装

### 启动桌面开发模式

```bash
wails dev
```

### 仅构建前端

```bash
cd frontend
npm run build
```

### 运行测试

```bash
go test ./...
```

### 构建桌面应用

```bash
wails build
```

## 安装与更新

现在项目已经支持通过 `make` 进行本地安装。

### 安装

```bash
make install
```

这个命令会同时完成两件事：

- 构建桌面应用并安装 `TypeLens.app`
- 构建 CLI 并安装 `typelens`

macOS 默认安装位置：

- 桌面应用：
  - 如果 `/Applications` 可写，则安装到 `/Applications/TypeLens.app`
  - 否则安装到 `~/Applications/TypeLens.app`
- CLI：
  - `~/.local/bin/typelens`

### 更新

```bash
make upgrade
```

`make upgrade` 会重新构建桌面应用和 CLI，然后覆盖安装到原来的位置。

### 卸载

```bash
make uninstall
```

会删除：

- 已安装的 `TypeLens.app`
- 已安装的 `typelens` CLI 可执行文件

### 自定义安装路径

如果你想改安装位置，可以在执行时覆盖变量：

```bash
make install INSTALL_APP_DIR=~/Applications INSTALL_BIN_DIR=/usr/local/bin
```

## 补充说明

- 导入文件的格式是“每行一个词”。
- “自动导入”里显示的来源标签主要用于前端展示和定位目录，不是词典语义本身的一部分。
- 后台同步未完成前，待同步词可能会继续显示在列表里。

英文版主文档见 [README.md](./README.md)。
