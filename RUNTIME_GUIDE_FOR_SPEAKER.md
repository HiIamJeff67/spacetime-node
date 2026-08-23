# Spacetime Node Runtime 導覽

這份文件提供講者快速理解 Spacetime Node 展示系統的 runtime、服務責任、資料流，以及重要程式碼位置。

## 一分鐘版本

Spacetime Node 是一個「捷運進站情境 → 個人化優惠推薦 → 兌換 → 成效分析」的 MVP。

```text
Frontend / Cloudflare Pages
        │ HTTP
        ▼
gateway-service
        │
        ├── PostgreSQL：交易與推薦真實來源
        ├── Kafka：非同步領域事件
        └── mobility-service：Beacon 解析與 Redis cache

Kafka
  ├── journey.entered.v1 → recommendation-service
  ├── recommendation.created.v1 → notification / analytics
  ├── recommendation.clicked.v1
  ├── recommendation.dismissed.v1
  └── redemption.succeeded.v1

recommendation-service
  ├── Redis：使用者偏好快取
  ├── Qdrant：優惠候選向量搜尋
  └── PostgreSQL：資格驗證、推薦保存
```

## Runtime 分類

1. 使用者瀏覽器中的 React frontend。
2. Docker Compose 啟動的 Go API 與 background workers。
3. PostgreSQL、Kafka、Redis、Qdrant、ClickHouse 等基礎資料元件。

Docker 使用同一份 `Dockerfile`，透過 `SERVICE` build argument 選擇不同的 `cmd/*` executable。

## Go runtimes

### gateway-service

入口：`cmd/gateway/main.go`；HTTP `8000`；gRPC `9000`。

Gateway 是瀏覽器的主要後端入口，負責建立進站旅程、接收 Beacon observation、查詢最新推薦、儲存使用者偏好、接收推薦互動、提供兌換與通知訂閱 API，以及啟動 PostgreSQL outbox publisher。

主要實作：

- `internal/journey/service.go`
- `internal/user/service.go`
- `internal/redemption/api.go`
- `internal/notification/api.go`

Gateway 不負責計算推薦，也不直接呼叫外部 Beacon provider。

```text
Beacon observation 或 station_id
    → mobility resolver
    → 建立 journeys
    → 同一個 transaction 寫入 outbox_events
    → outbox publisher 發送 journey.entered.v1
```

### mobility-service

入口：`cmd/mobility/main.go`；HTTP `8001`；gRPC `9001`。

Mobility 是 Beacon 的 anti-corruption layer，將外部 Beacon API 格式轉成內部站點情境。

主要檔案：

- `internal/mobility/provider.go`
- `internal/mobility/resolver.go`
- `internal/mobility/service.go`

輸入是 `UUID / Major / Minor / Power`，輸出是 `station_id / line_id / position_id / station_name / confidence`。

Resolver 的目前順序是：

```text
process-local hot cache
    → Redis normalized context cache
    → Beacon provider
    → sanitized fixture
    → station-level fallback
```

Provider 成功解析的 context 會寫入 Redis，key 格式為：

```text
mobility:beacon:{uuid}:{major}:{minor}
```

預設 TTL 為 15 分鐘。Redis 是可重建資料；Redis 暫時不可用時不會阻斷 provider 或 fixture fallback。

`provider.go` 負責呼叫外部 Beacon API；`resolver.go` 負責解析 `SID`、`LID`、`POSINO`、`POSITION`、站點 normalization、cache 與 fallback。

### recommendation-service

入口：`cmd/recommendation/main.go`；HTTP `8002`；gRPC `9002`。

它主要是 Kafka consumer，而不是前端直接查詢推薦的 API。它消費 `journey.entered.v1` 並非同步產生推薦；前端查詢最新推薦時，是透過 Gateway 讀 PostgreSQL。

消費流程位於 `internal/recommendation/consumer.go`：

1. 讀取並驗證 `journey.entered.v1`。
2. 避免同一 journey replay 重複產生推薦。
3. 讀取使用者偏好與學習到的分類權重。
4. 產生 profile embedding。
5. 向 Qdrant 搜尋候選優惠。
6. 回 PostgreSQL 驗證站點、有效期、庫存、點數與兌換狀態。
7. 依規則排序並產生文案。
8. 保存 `recommendations`、candidate summary 與 `recommendation.created.v1`。

核心程式位於 `internal/recommendation/recommendation.go` 的 `Recommend()`。

排序會考慮 Qdrant vector score、預測目的地、點數預算、庫存、偏好類別，以及過去 click／dismiss／redemption 累積的分類權重。

### `recommendation/copy.go`

這裡的 `copy` 指推薦文案，不是複製檔案。它把後端已確認的事實轉成使用者看得懂的文字。

主要函式：

- `TemplateCopy()`：固定模板文案。
- `GenerateCopy()`：選擇 template 或外部 LLM provider。
- `ValidateCopyFacts()`：確認輸入事實完整且長度合法。
- `ValidateCopyOutput()`：防止 LLM 改寫優惠名稱、點數、折扣或產生未授權數字。
- `NewHTTPCopyGenerator()`：呼叫 OpenAI-compatible `/v1/chat/completions`。

如果 LLM timeout、格式錯誤或內容不符合 facts，會退回 template。因此 `copy.go` 只負責「怎麼說」，不負責「推薦哪一張券」。

### embedding-indexer

入口：`cmd/embedding-indexer/main.go`。

這是 background worker，負責建立 Qdrant collection、讀取 active offers、產生 embedding、寫入 Qdrant，以及消費 `offer.changed.v1` 更新或刪除 vector。

主要檔案：

- `internal/recommendation/indexer.go`
- `internal/recommendation/embedder.go`
- `internal/recommendation/qdrant.go`
- `internal/recommendation/qdrant_index.go`

預設 `demo-hash-v1` 是 deterministic hash embedding，讓 demo 不依賴 GPU 或外部模型。未來可用 `EMBEDDING_MODE=http` 改接 OpenAI-compatible embedding service。

### redemption-service

入口：`cmd/redemption/main.go`；HTTP `8003`；gRPC `9003`。

它負責真正的兌換交易：鎖定使用者與 inventory、驗證 idempotency、檢查一次性兌換、扣點、扣庫存、寫 points ledger，最後發出 `redemption.succeeded.v1`。

主要實作：`internal/redemption/service.go`。

### notification-worker

入口：`cmd/notification/main.go`。

它消費 `recommendation.created.v1`，依使用者通知設定與時間範圍決定是否發送通知，並寫入 sent／delivered／failed event。預設是 deterministic mock；設定 VAPID keys 與 `NOTIFICATION_PROVIDER=webpush` 才會發送真正的 Web Push。

主要檔案：`internal/notification/worker.go`、`provider.go`、`service.go`。

### analytics-consumer

入口：`cmd/analytics/main.go`。

它消費 journey、recommendation、dismiss、redemption 等產品事件並寫入 ClickHouse，供 Grafana 顯示漏斗與 KPI。ClickHouse 是可由 Kafka 重建的分析 projection，不是交易真實來源。

主要檔案：`internal/analytics/consumer.go`、`events.go`、`clickhouse.go`。

## 共用 platform

- `internal/platform/app`：Kratos HTTP/gRPC、health、readiness、version、CORS、OpenTelemetry 與 graceful shutdown。
- `internal/platform/config`：集中解析 runtime environment variables。
- `internal/platform/outbox`：確保 PostgreSQL transaction 與 Kafka event 不脫鉤。
- `api/proto/spacetime_node/v1`：HTTP、gRPC、OpenAPI 與錯誤碼的 contract source of truth。

## 基礎資料元件

| 元件 | 用途 | 是否為真實來源 |
|---|---|---|
| PostgreSQL | 使用者、旅程、優惠、庫存、推薦、兌換、點數 ledger | 是 |
| Kafka | 非同步領域事件與 consumer group | 事件來源 |
| Redis | 使用者偏好與 normalized Beacon context 的短期快取 | 否，可重建 |
| Qdrant | 優惠向量與候選召回 | 否，可重建 |
| ClickHouse | 產品事件與轉換漏斗 | 否，可重建 |
| Grafana / OTel LGTM | Dashboard、logs、metrics、traces | 否 |

## Frontend runtime

Frontend 位於相鄰的 `spacetime-node-app` repo，主要檔案是 `src/App.tsx`、`src/api.ts` 與 `src/beacon.ts`。

### `src/api.ts`

集中處理 profile、preferences、station entry、Beacon entry、latest recommendation、click／dismiss、redemption 與 Web Push subscription 的 HTTP request。

### `src/beacon.ts`

只負責瀏覽器端 Web Bluetooth：啟動 Chrome Beacon scan、解析 Apple iBeacon 的 UUID／Major／Minor／Power，再把 observation 傳給 Gateway。它本身不判斷捷運站，站點解析由 backend 的 mobility-service 完成。

### `src/App.tsx`

負責 onboarding、Demo Map、Beacon 掃描、recommendation polling、優惠詳情、dismiss 與 redemption。`loadRecommendation()` 建立 entry event 後輪詢最新推薦；`dismissRecommendation()` 發送 dismissed event 後重新抓取同一 journey 的推薦。

## 完整展示流程

```text
使用者選偏好
    → Chrome Beacon 或 Demo 站點
    → Gateway 建立 journey
    → PostgreSQL outbox
    → Kafka journey.entered.v1
    → recommendation-service
    → Qdrant 找候選
    → PostgreSQL 驗證與排序
    → copy.go 產生文案
    → Gateway 查詢最新推薦
    → 前端顯示最多 10 張優惠
    → 點擊／dismiss／兌換
    → Kafka
    → ClickHouse dashboard
```

## 講者版本

> 這是一個以捷運進站事件為觸發點的個人化優惠推薦 MVP。前端可以使用 Demo 站點或 Chrome Web Bluetooth 取得 Beacon observation；Gateway 將進站事件寫入 PostgreSQL outbox，再透過 Kafka 非同步觸發 recommendation-service。推薦服務使用使用者偏好、歷史互動、站點、點數與庫存條件，搭配 Qdrant 候選召回與規則排序，產生最多十張可用優惠。兌換由 PostgreSQL transaction 保證點數、庫存與一次性兌換的一致性，最後由 analytics-consumer 將事件投影到 ClickHouse 做成效分析。
