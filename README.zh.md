# new-api（精简版）

本仓库是基于上游项目维护的**社区精简版本**：
- 上游项目：[QuantumNous/new-api](https://github.com/QuantumNous/new-api)

## 重要说明
- 本仓库**不是**上游官方仓库。
- 本仓库用于个人/工程场景下的精简维护分支。
- 如需官方完整功能、发布节奏与路线图，请以官方仓库为准。

## 本精简版目标
- 保留核心网关能力，降低维护复杂度。
- 面向当前部署需求，减少非必要模块。
- 提高代码可读性与可控性。

## 快速开始

```bash
git clone https://github.com/Xuuuuu04/new-api.git
cd new-api
cp .env.example .env
# 按需修改 .env
docker compose up -d --build
```

启动后访问：`http://localhost:3000`

## 合规与责任
- 请遵守模型供应商服务条款。
- 请遵守你所在地区的法律法规。
- 生产部署中的安全、数据治理与稳定性由使用者自行负责。

## 许可证
本仓库遵循 `LICENSE` 文件中的许可条款。

## 致谢
感谢 `QuantumNous/new-api` 上游维护者与贡献者。
