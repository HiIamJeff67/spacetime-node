# Spacetime Node（時空節點）

「時空節點」後端 MVP。系統以捷運進站情境觸發個人化優惠推薦，並可完成兌換、商家核銷與成效分析。

目前已完成 Sprint 0 基線、Sprint 1 的推薦／索引核心與 Sprint 2 的兌換、分析 projection、LGTM telemetry；四個 API 服務、embedding indexer 與 analytics consumer 可由單一 Docker Compose 啟動。

## Local start

先建立只供本機使用的設定檔，並把 PostgreSQL 密碼替換成自己的本機值：

```bash
cp .env.example .env
```

接著以單一 Compose 啟動四個 API 服務、embedding indexer／analytics workers 與 Kafka（KRaft）、PostgreSQL、Redis、ClickHouse、Qdrant：

```bash
docker compose --env-file .env -f deploy/compose/compose.yaml up --build
```

在空的 PostgreSQL volume 上，Compose 會依序執行 [`migrations/postgres/`](migrations/postgres/) 內的 schema 與 demo seed。這是初始 baseline；重新建立資料請先閱讀該目錄的說明，因為操作會刪除本機資料卷。

HTTP 健康檢查：

```bash
curl http://localhost:8000/healthz
curl http://localhost:8000/readyz
curl http://localhost:8000/version
curl http://localhost:8001/healthz
curl http://localhost:8002/healthz
curl http://localhost:8003/healthz
# Grafana: http://localhost:3000 (admin / admin by default)
# LGTM Grafana: http://localhost:3001 (development observability stack)
```

`/healthz` 只表示 process 還活著；`/readyz` 會檢查該服務必要的 PostgreSQL／Qdrant 依賴，未就緒時回傳 HTTP 503；`/version` 只回報服務版本，不再充當 readiness probe。

### Browser deployment and CORS

The gateway accepts browser requests only from the exact origins listed in
`CORS_ALLOWED_ORIGINS` (comma-separated, for example
`https://demo.example.com`). It handles `OPTIONS` preflight for the public API
and returns `403` for an unknown origin; leave the value empty for curl-only
local development. The frontend's `VITE_API_BASE_URL` is public configuration
and must point to the HTTPS gateway URL; database, Kafka, Redis, Qdrant and
observability credentials stay server-side.

The gateway exposes the browser-facing entry, user, recommendation, and
redemption routes on one origin. The dedicated redemption service remains
available on port `8003` for internal/service-level separation.

For a demo deployment, build the frontend container from
`../spacetime-node-app` with `VITE_API_BASE_URL` set, terminate TLS at the
hosting ingress, and configure the same frontend origin in the backend's
`CORS_ALLOWED_ORIGINS`. Probe `/healthz` for process health and `/readyz` for
dependency readiness before routing traffic.

服務也可獨立啟動，並提供 `/healthz` 與 `/version`：

```bash
go run ./cmd/gateway
go run ./cmd/mobility
go run ./cmd/recommendation
go run ./cmd/redemption
```

預設 HTTP ports 為 `8000` 至 `8003`，內部 gRPC ports 為 `9000` 至 `9003`。使用 `SERVICE_NAME`、`HTTP_ADDR` 與 `GRPC_ADDR` 可覆寫；單一服務的安全設定範本在 [`configs/local.env.example`](configs/local.env.example)。

所有 service 的環境變數一律由 `internal/platform/config` 載入；它集中管理 runtime ports 及 Kafka、PostgreSQL、Redis、ClickHouse、Qdrant、LLM 的連線設定，避免各服務各自解析環境變數。

Embedding 預設使用自包含的 `demo-hash-v1`；要切換語意模型，設定 `EMBEDDING_MODE=http`、`EMBEDDING_BASE_URL`、`EMBEDDING_MODEL`、實際向量維度與新的 `EMBEDDING_COLLECTION`（例如 `offer_embeddings_v2`）。服務期待 OpenAI-compatible `POST /v1/embeddings`，未設定時不會依賴外部模型服務。

要啟用真實 Web Push，先產生一次 VAPID key pair：

```bash
go run ./cmd/vapid
```

將 private key 只放在 backend `.env`，public key 同時放在 backend 的 `VAPID_PUBLIC_KEY` 與 frontend 的 `VITE_VAPID_PUBLIC_KEY`；再將 backend `NOTIFICATION_PROVIDER` 設為 `webpush`。

要補齊本機 demo 的曝光／點擊漏斗，可在 Compose 啟動後執行：

```bash
KAFKA_BROKERS=localhost:29092 go run ./cmd/analytics-demo \
  -journey-id <journey_id> -recommendation-id <recommendation_id>
```

這個命令只發送可重播的 engagement mock events，不會修改點數、庫存或兌換資料。

完整端對端流程可直接執行：

```bash
./scripts/demo.sh
```

腳本會建立進站事件、等待非同步推薦、兌換推薦優惠並呼叫商家核銷 mock；可用 `GATEWAY_URL`、`REDEMPTION_URL`、`STATION_ID`、`MAX_ATTEMPTS` 等環境變數覆寫展示參數。

Web Demo 完成 onboarding 後會以第一個選取站點建立 entry event；推薦服務再以優惠的結構化
`category` 與使用者偏好比對，不依賴中文標題或描述文字碰巧相符。點擊、標記沒興趣與兌換會累積分類權重，影響下一次排序並留下可解釋 reason；未選站時才使用 R04 作為展示 fallback。
目前 demo catalog 涵蓋 R04、R03、R09、G12、G07、BL18、BL12、O09，並提供咖啡、午餐、甜點、飲品、速食、便利商店與生活百貨等展示券；既有 VM 請依 [`migrations/postgres/README.md`](migrations/postgres/README.md) 套用最新 catalog migration。

SCRUM-13 的完整交付 runbook、驗收證據與 failure boundary 請見 [`docs/delivery.md`](docs/delivery.md)。

公開 Demo 的 Google Cloud VM、Cloudflare Pages、CORS 與 Quick Tunnel／tmux
部署步驟請見 [`docs/demo-deployment.md`](docs/demo-deployment.md)。

API contract 改動後執行 `make proto`，重新產生 Go、gRPC、Kratos HTTP 與標準錯誤碼 stub；它使用 `go.mod` 鎖定的 Kratos v3 Proto include。

提交前可執行 `make check`，一次完成 `go mod tidy -diff`、`go test ./...`、`go vet ./...` 與 Compose config validation。

執行 `make openapi` 可由相同 Proto 產生 [`api/openapi/openapi.yaml`](api/openapi/openapi.yaml)。Kafka event contract 位於 [`api/events/v1/`](api/events/v1/)，CopyGenerator 的 facts input / output JSON Schema 位於 [`api/schemas/`](api/schemas/)。

停止並保留資料卷：`docker compose --env-file .env -f deploy/compose/compose.yaml down`。若要連同本機資料刪除，明確加上 `--volumes`。

目前完成度、Beacon 串接計畫、embedding 演進與完整資料流請見 [docs/architecture.md](docs/architecture.md)；目錄與程式碼責任邊界見 [.github/ARCHITECTURE.md](.github/ARCHITECTURE.md)。
