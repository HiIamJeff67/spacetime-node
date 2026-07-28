# Spacetime Node（時空節點）

「時空節點」後端 MVP。系統以捷運進站情境觸發個人化優惠推薦，並可完成兌換、商家核銷與成效分析。

目前為 Sprint 0。四個服務已有可執行的 Kratos HTTP/gRPC 骨架；基礎設施可由單一 Docker Compose 啟動，業務功能尚未實作。

## Local start

先建立只供本機使用的設定檔，並把 PostgreSQL 密碼替換成自己的本機值：

```bash
cp .env.example .env
```

接著以單一 Compose 啟動四個服務與 Kafka（KRaft）、PostgreSQL、Redis、ClickHouse、Qdrant：

```bash
docker compose --env-file .env -f deploy/compose/compose.yaml up --build
```

HTTP 健康檢查：

```bash
curl http://localhost:8000/healthz
curl http://localhost:8001/healthz
curl http://localhost:8002/healthz
curl http://localhost:8003/healthz
```

服務也可獨立啟動，並提供 `/healthz` 與 `/version`：

```bash
go run ./cmd/gateway
go run ./cmd/mobility
go run ./cmd/recommendation
go run ./cmd/redemption
```

預設 HTTP ports 為 `8000` 至 `8003`，內部 gRPC ports 為 `9000` 至 `9003`。使用 `SERVICE_NAME`、`HTTP_ADDR` 與 `GRPC_ADDR` 可覆寫；單一服務的安全設定範本在 [`configs/local.env.example`](configs/local.env.example)。

所有 service 的環境變數一律由 `internal/platform/config` 載入；它集中管理 runtime ports 及 Kafka、PostgreSQL、Redis、ClickHouse、Qdrant、LLM 的連線設定，避免各服務各自解析環境變數。

停止並保留資料卷：`docker compose --env-file .env -f deploy/compose/compose.yaml down`。若要連同本機資料刪除，明確加上 `--volumes`。

架構與目錄責任請見 [.github/ARCHITECTURE.md](.github/ARCHITECTURE.md)。
