# new-api (Simplified Edition)

This repository is a **community-maintained simplified edition** based on the upstream project:
- Upstream: [QuantumNous/new-api](https://github.com/QuantumNous/new-api)

## Important Notice
- This repository is **NOT** the official upstream repository.
- It is a simplified branch for personal/engineering usage and focused scenarios.
- For official releases, full features, and upstream roadmap, please follow the upstream project.

## What This Edition Focuses On
- Keep a smaller and easier-to-maintain code surface.
- Retain core gateway capabilities needed by this deployment.
- Remove or trim non-essential modules for this branch.

## Quick Start

```bash
git clone https://github.com/Xuuuuu04/new-api.git
cd new-api
cp .env.example .env
# edit .env
docker compose up -d --build
```

Then open: `http://localhost:3000`

## Compatibility & Responsibility
- Please comply with your model providers' Terms of Service.
- Please comply with local laws and regulations before any production usage.
- You are responsible for your own deployment, security hardening, and data governance.

## License
This repository follows the license declared in `LICENSE`.

## Credits
Thanks to the upstream maintainers and contributors of `QuantumNous/new-api`.

## Language
- 中文：[`README.md`](./README.md)
- English：[`README_EN.md`](./README_EN.md)

## 统一源码目录
- 源码入口：[`src/`](./src)

## 目录结构
- 结构说明：[`docs/PROJECT_STRUCTURE.md`](./docs/PROJECT_STRUCTURE.md)

## 迁移说明
- 核心目录已迁移到 `src/` 下。
- 根目录保留兼容软链接，历史命令与路径可继续使用。
