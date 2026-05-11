---
name: meshhub-project-rules
description: 当在 MeshHub 项目中创建、修改、构建或发布代码时使用本技能。要求先阅读项目内所有相关 skill，并严格遵守前端、样式、后端、服务端、Wails 构建产物和导出目录的放置规范。
---

# MeshHub 项目总规范

## 启动前必须读取的 Skill

在本项目中写代码前，必须先读取相关项目 skill。

最低要求：

- 前端 Vue 代码：先读 `Frotend_Vue/frontend-code-style-skill/SKILL.md`。
- Go、Wails、server 后端代码：先读 `backend_go/backend-code-style-skill/SKILL.md`。
- 同时涉及前后端时：两个 skill 都要读。

本总规范只负责项目边界和目录归属；具体代码风格以对应前端或后端 skill 为准。

## 项目根目录

项目根目录固定为：

```text
asset-transcoder/
```

当前项目放在桌面：

```text
/Users/jhb/Desktop/asset-transcoder
```

不要把项目代码写到桌面其它位置，也不要写到用户主目录散落文件。

## 目录职责

项目目录职责固定如下：

```text
asset-transcoder/
├── Frotend_Vue/                 # Vue 前端源码，只放前端 Vue/JS 配置和前端专属 skill
├── style/                       # Tailwind 和全局 CSS
├── backend_go/                  # Go + Wails 桌面应用后端
├── server/                      # 服务端能力代码，例如模型转换、任务队列、缓存处理
├── export_app/                  # Wails 最终发布导出产物
├── scripts/                     # 构建、导出、自动化脚本
└── asset-transcoder-project-skill/ # 项目总规范 skill
```

## 前端文件放置规则

Vue 前端源码只允许放在：

```text
Frotend_Vue/
```

要求：

- `.vue` 文件只能创建在 `Frotend_Vue/` 内部。
- 前端入口、Vite 配置、前端 package 配置放在 `Frotend_Vue/`。
- 前端依赖使用 `pnpm` 管理。
- 不要把 Vue 文件放到 `style/`、`backend_go/`、`server/` 或项目根目录。

前端代码风格必须遵守：

```text
Frotend_Vue/frontend-code-style-skill/SKILL.md
```

## 样式文件放置规则

Tailwind 和 CSS 样式统一放在：

```text
style/
```

当前 Tailwind 入口为：

```text
style/tailwind/index.css
```

要求：

- 全局样式、Tailwind 入口、通用 CSS 放在 `style/`。
- 只有组件局部且确实需要贴近组件时，才允许放在 Vue 单文件组件内部。
- 不要在 `backend_go/` 或 `server/` 中写前端样式文件。

## Go + Wails 后端放置规则

Wails 桌面应用后端放在：

```text
backend_go/
```

要求：

- Wails 入口、Go module、Wails 配置放在 `backend_go/`。
- 前端通过 Wails 调用的 Go 方法放在 `backend_go/`。
- Go 后端代码风格必须遵守 `backend_go/backend-code-style-skill/SKILL.md`。
- 不要把 Wails Go 入口写到 `server/` 或项目根目录。

## Server 服务端放置规则

独立服务端能力放在：

```text
server/
```

适合放在这里的内容：

- 三维模型格式转换。
- STEP/STP 解析或转换调度。
- SolidWorks 原生格式转换适配。
- 转换任务队列。
- 临时文件和缓存处理。
- 服务端 API、RPC 或后台任务。

如果某段 Go 代码是 Wails 桌面端生命周期或前端绑定，放 `backend_go/`。

如果某段代码是独立服务端处理能力，放 `server/`。

## Wails 构建产物规则

前端构建产物给 Wails 嵌入时，输出到：

```text
backend_go/frontend_dist/
```

最终 Wails 发布导出产物放到：

```text
export_app/
```

要求：

- `backend_go/frontend_dist/` 是 Wails 嵌入前端资源的中间产物目录。
- `backend_go/build/` 是 Wails 构建过程目录，不作为最终交付目录。
- `export_app/` 是最终给用户使用或分发的应用导出目录。
- 不要把最终发布产物放在桌面、项目根目录或 `Frotend_Vue/`。

当前导出脚本为：

```text
scripts/export-wails-build.sh
```

构建发布流程应使用：

```bash
pnpm wails:build
```

该流程应完成：

1. 构建前端。
2. 在 `backend_go/` 中运行 Wails 构建。
3. 把 Wails 构建结果复制到 `export_app/`。

## 常用命令

安装依赖：

```bash
pnpm install
```

前端开发：

```bash
pnpm frontend:dev
```

前端构建：

```bash
pnpm frontend:build
```

Wails 开发：

```bash
pnpm wails:dev
```

Wails 发布构建：

```bash
pnpm wails:build
```

Go 编译验证：

```bash
cd backend_go && go build ./...
```

## 修改后的验证要求

根据改动范围执行验证：

- 改前端：运行 `pnpm frontend:build`。
- 改 Go/Wails：运行 `cd backend_go && go build ./...`。
- 改 Wails 发布流程：运行 `pnpm wails:build`，并确认产物进入 `export_app/`。
- 改前后端交互：前端构建和 Go 编译都要跑。

如果本机没有 `wails` 命令，要明确说明无法完成 Wails 发布构建。

## Git 处理规则

如果当前目录是 git 仓库，改完代码后按项目要求处理 pull 和 push。

如果当前目录不是 git 仓库，要明确说明无法执行 pull/push，不要假装已经同步。
