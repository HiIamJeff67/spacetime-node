# Spacetime Node（時空節點）後端 MVP 架構文件

> **目前程式碼服務邊界與目錄結構以 [`.github/ARCHITECTURE.md`](../.github/ARCHITECTURE.md) 為準。** 本文件保留完整的 MVP 設計脈絡；其中較早的三服務描述已由 `mobility-service` 的外部捷運 API 邊界補充。

> 版本：0.1  
> 狀態：提案／MVP 實作基線  
> 專案代號：`spacetime-node`  
> 對應 Jira Epic：[SCRUM-5](https://hajimi-o.atlassian.net/browse/SCRUM-5)

## 1. 專案目標

「時空節點」是一個面向台北捷運通勤情境的個人化點數推薦後端。系統在使用者進站事件發生後，結合預先計算的偏好、站點、時段與可兌換優惠，在兩秒內產生可解釋的推薦；使用者接著可完成兌換，並將曝光、點擊、兌換、核銷等資料轉化成可驗證的商業成效。

本專題的首要目標是完成可穩定展示的後端 MVP，而不是模擬完整北捷正式系統。

### MVP 成功條件

- 可用一個 `docker compose up` 啟動所有 demo 所需元件。
- 可完成「進站 → 推薦 → 兌換 → 核銷模擬 → 成效 dashboard」的端對端流程。
- 在本機受控 demo 環境中，進站至推薦結果產生的 P95 延遲小於 2 秒。
- 每個關鍵動作都能以 `journey_id`、`recommendation_id`、`event_id` 與 `trace_id` 追溯。
- Dashboard 同時呈現技術健康度與商業轉換漏斗。

### 非目標

下列項目不屬於 MVP 必要範圍，僅保留可擴充介面或 mock：

- 真實北捷閘門、會員、票務與站點 API。
- FCM/APNs 真實推播、真實商家 POS 與實體到店定位。
- 完整向量資料庫、RAG、embedding 訓練與 GPU 部署。
- 多區高可用、Kubernetes、生產級 Kafka 叢集。
- 面向終端使用者的完整前端。

## 目前實作狀態（2026-08-13）

目前不是只有基礎設施空殼：進站 fixture、推薦、兌換、核銷 mock、分析與可觀測性已形成可執行的垂直切片。Web Push 與 Beacon resolver 同時保留 deterministic mock 與可選的真實 provider adapter。

| 能力 | 狀態 | 現況與邊界 |
|---|---|---|
| Compose 與服務基線 | 已完成 | Gateway、mobility、recommendation、redemption、embedding indexer、analytics consumer，以及 Kafka、PostgreSQL、Redis、ClickHouse、Qdrant、LGTM 可共同啟動 |
| 進站觸發推薦 | 已完成 demo 版 | App／腳本直接提供 `station_id`、`line_id`、`position_id`；Gateway 建立 journey 與 outbox event，Kafka 非同步觸發推薦 |
| 推薦核心 | 已完成 demo 版 | Qdrant 召回候選，PostgreSQL 驗證站點、有效期、庫存與點數，規則排序後保存推薦與 explainability |
| 兌換與核銷 | 已完成 | 原子扣點／扣庫存、`Idempotency-Key`、transactional outbox、狀態查詢與商家核銷 mock |
| 分析與可觀測性 | 已完成基線 | ClickHouse product-event projection、Grafana dashboard、OpenTelemetry traces／metrics／logs |
| Runtime readiness | 已完成基線 | `/healthz`、dependency-aware `/readyz`、`/version` 語義分離；Kratos graceful shutdown 有 10 秒上限 |
| Beacon 進站解析 | 已完成 fixture/provider MVP | `mobility-service` 提供 `/v1/mobility/beacon/resolve`；以 SID+LID normalization、短期記憶體 cache、sanitized fixture 與 provider timeout fallback 建立位置 context；Gateway 仍可選擇使用此 API，尚未強制綁定真實 Beacon |
| 使用者常用站／消費偏好 | 已完成 demo API | PostgreSQL 保存 profile／偏好，Gateway 提供 `/v1/users/me` 與 `/v1/users/me/preferences`；Redis 只保存可重建摘要 |
| 手機推播 | 已完成 mock + Web Push MVP | Gateway 提供訂閱註冊／撤銷；PostgreSQL 保存 subscription、worker 依使用者 timezone／時段篩選 `recommendation.created.v1`，可用 VAPID provider 發送加密瀏覽器通知；push service 接受後記錄 sent，mock 才產生 delivered |
| Embedding | 已完成可切換 adapter | 預設 `demo-hash-v1` 保持 demo 自包含；設定 `EMBEDDING_MODE=http` 可讓 offer 與「偏好＋站點情境」共用 OpenAI-compatible semantic embedding，並切換到獨立 Qdrant collection |

```mermaid
flowchart LR
    App["App / Demo script"] -->|"目前：直接提供 station_id"| Gateway["gateway-service"]
    Beacon["捷運 Beacon observation"] -->|"UUID / Major / Minor / Power"| Mobility["mobility-service\nBeacon resolver"]
    Mobility -->|"normalized station context"| Gateway
    Gateway --> PG["PostgreSQL + outbox"]
    PG --> Kafka["Kafka"]
    Kafka --> Recommendation["recommendation-service"]
    Recommendation --> Qdrant["Qdrant candidates"]
    Recommendation --> PG
    Kafka --> Analytics["analytics-consumer"]
    Analytics --> ClickHouse["ClickHouse"]
    App --> Redemption["redemption-service"]
    Redemption --> PG

    classDef complete fill:#d7f5df,stroke:#238636,color:#111;
    classDef partial fill:#fff3bf,stroke:#9a6700,color:#111;
    classDef planned fill:#f2f2f2,stroke:#6e7781,stroke-dasharray:5 5,color:#111;
    class Gateway,PG,Kafka,Recommendation,Qdrant,Analytics,ClickHouse,Redemption complete;
    class App partial;
    class Beacon,Mobility planned;
```

## 2. 核心設計原則

1. **MVP 優先，但保留正確演進路徑**：只實作 demo 需要的能力；服務邊界、事件契約和可觀測性則依正式系統原則設計。
2. **Contract-first**：HTTP、gRPC 與 Kafka 的資料格式先定義，再寫實作；Proto 與版本化事件 schema 是跨服務的真實契約。
3. **交易與事件分責**：PostgreSQL 是點數、庫存、兌換的強一致性真實來源；Kafka 是可重播的領域事件紀錄。
4. **可解釋推薦優先於模型複雜度**：向量檢索只召回候選，規則排序決定最終優惠；LLM 只能根據已驗證 facts 美化文案，不能決定優惠、點數、資格、庫存或交易結果。
5. **商業成效與技術維運分流**：產品事件進 ClickHouse 做轉換分析；logs、metrics、traces 進 LGTM 做系統維運。
6. **隱私預設保護**：事件、logs、traces 不寫入原始使用者識別資料；使用 hash 或受控的 surrogate ID。

## 3. 系統總覽

```mermaid
flowchart LR
    D["Demo CLI / 極小前端"] -->|HTTP| G["Gateway Service"]
    G -->|publish| K["Apache Kafka"]
    K -->|offer.changed.v1| I["Embedding Indexer"]
    I -->|upsert| VDB["Qdrant\n向量索引"]
    K -->|journey.entered.v1| R["Recommendation Service"]
    R -->|read cached preference| Redis["Redis"]
    R -->|top-K offer IDs| VDB
    R -->|read offers / write recommendation| PG[("PostgreSQL")]
    R -->|optional, deadline-bound facts| LLM["輕量開源 LLM\n文案 adapter"]
    R -->|publish recommendation.created.v1| K
    D -->|HTTP| X["Redemption Service"]
    X -->|transaction + outbox| PG
    PG -->|outbox publisher| K
    K -->|business events| A["Analytics Consumer"]
    A --> CH[("ClickHouse")]
    CH --> Grafana["Grafana 商業 Dashboard"]
    G --> OTel["OpenTelemetry Collector"]
    R --> OTel
    X --> OTel
    OTel --> LGTM["Loki / Tempo / Mimir / Grafana"]
```

## 4. 服務邊界

MVP 採用四個 API 服務與獨立 worker；不依照原始構想拆成七個服務，避免單人開發時把成本耗在網路呼叫、設定與部署上。

| 服務 | 責任 | 對外／對內介面 | MVP 不做什麼 |
|---|---|---|---|
| `gateway-service` | 接收模擬進站、提供查詢推薦等 BFF API、建立旅程識別 | HTTP；發布 Kafka | 真實閘門／北捷會員介接 |
| `mobility-service` | 封裝 Beacon provider、SID+LID normalization、cache 與 fixture fallback | `POST /v1/mobility/beacon/resolve`；亦提供 gRPC | 不讓其他服務直接持有捷運 provider 帳密或格式 |
| `recommendation-service` | 讀取偏好快取、向量召回候選、規則篩選與排序、保存推薦 | Kafka consumer、gRPC／HTTP（內部） | 真正模型訓練、讓 LLM 決策 |
| `embedding-indexer` | 消費優惠變更事件、產生 embedding、更新 Qdrant 可重建索引 | Kafka consumer | 正式模型訓練與 GPU serving |
| `redemption-service` | 點數扣除、庫存保留、兌換單、核銷 mock、outbox | HTTP／gRPC；發布 Kafka | 真實 POS、支付與退款流程 |
| `analytics-consumer` | 消費產品事件、寫入 ClickHouse、產出漏斗資料 | Kafka consumer | 線上推薦決策 |

`analytics-consumer` 可以先作為獨立 worker，而不是完整 Kratos 對外服務。這樣仍能展示 consumer group、事件解耦和資料投影。

### Beacon 串接狀態與預計流程

團隊口語中的「進站 becam」應是 **Beacon**。`mobility-service` 現在提供 Beacon resolver；現有 `POST /v1/entry-events` 仍由呼叫端直接傳入內部 `station_id`，因此真實 Beacon 不會阻塞既有 demo 流程。

[SCRUM-38](https://hajimi-o.atlassian.net/browse/SCRUM-38) 記錄的目標來源是 `GetBeaconInfo`：輸入 UUID、Major、Minor、Power 與執行期帳密，回傳 BID、SID、LID、POSINO、POSITION、STATIONID 與站名。預計只由 `mobility-service` 呼叫外部服務，並遵守以下邊界：

- 使用 `SID + LID` 映射內部 `station_id`，保留轉乘線別；不可直接把外部 `STATIONID` 當內部主鍵。
- `POSINO`／`POSITION` 只提供站內位置或鄰近出口加分，不是到店、扣點或兌換資格證據。
- 捷運帳密只存在 runtime secret，不進 Git、Kafka、log 或 trace。
- 外部 API timeout 或無法使用時，退回 cache 或 sanitized station-level fixture；推薦與兌換不能被外部 API 阻塞。

```mermaid
sequenceDiagram
    participant App as App
    participant G as gateway-service
    participant M as mobility-service
    participant C as Redis cache
    participant T as TRTC GetBeaconInfo
    participant P as PostgreSQL outbox
    participant K as Kafka

    App->>G: Beacon observation
    G->>M: Resolve UUID / Major / Minor / Power
    M->>C: Read normalized Beacon mapping
    alt Cache hit
        C-->>M: station / line / position context
    else Cache miss
        M->>T: POST GetBeaconInfo with runtime credential
        alt Valid response
            T-->>M: SID / LID / POSINO / POSITION
            M->>M: Normalize to internal station_id
            M->>C: Cache normalized context
        else Timeout or provider error
            M->>M: Use sanitized station-level fixture
        end
    end
    M-->>G: Normalized context + source + confidence
    G->>P: Commit journey + journey.entered.v1
    P->>K: Publish asynchronously
```

這張圖描述目前的 resolver 邊界；Gateway 尚未強制呼叫 resolver，正式 Beacon provider 與 Gateway 串接可在取得穩定外部契約後再開啟。

## 5. 技術選型

| 類別 | 選擇 | 角色與理由 |
|---|---|---|
| 語言 | Go | 單人可維護、併發處理適合事件 consumer、部署單純 |
| 微服務框架 | Kratos | Proto contract-first、HTTP / gRPC、middleware 與服務骨架 |
| 非同步事件 | Apache Kafka（KRaft） | 可保留、重播與分組消費的事件骨幹；MVP 使用單 broker |
| 交易資料庫 | PostgreSQL | 點數、庫存、兌換所需的交易、約束與冪等性 |
| 分析資料庫 | ClickHouse | 事件漏斗、分群、時間序列聚合與 dashboard 查詢 |
| 快取 | Redis | 預算偏好、去重、短期節流與熱資料 |
| 向量資料庫 | Qdrant | 優惠／商家語意候選召回；只保存可重建索引，不是真實交易來源 |
| LLM | 輕量開源 instruct model，經 adapter 呼叫 | 僅生成受控繁中推薦文案；可在本機或相容 API 執行 |
| API 規格 | Proto + OpenAPI / Swagger UI | 單一契約來源；避免手寫重複文件 |
| 可觀測性 | OpenTelemetry + LGTM | 從開發版 collector 演進至 Loki、Grafana、Tempo、Mimir |
| 本機部署 | Docker Compose | 評審可重現、環境依賴可控 |

### 資料來源責任

PostgreSQL 的交易異動先透過 transactional outbox 發成 Kafka 領域事件。`analytics-consumer` 消費事件後寫入 ClickHouse，因此 ClickHouse 是可重播重建的分析 projection，而非 recommendation service 的同步讀取來源。

Qdrant 同樣是可重建的衍生索引：`offer.changed.v1` 由 embedding indexer 消費，生成／更新優惠向量。推薦服務只從 Qdrant 取得候選 `offer_id`，再回 PostgreSQL 檢查即時庫存、有效期、點數與資格。

### 為何不把 MongoDB / Cassandra 放進 MVP

- **MongoDB**：若未來增加多輪聊天、非結構化 LLM 對話紀錄或外部原始文件，可作為選項；目前核心資料有明確交易一致性需求，並不適合當主資料庫。
- **Cassandra**：適用於極大量、固定查詢模式的分散式寫入場景。MVP 的點數交易與資料分析規模都不需要它，導入只會增加資料建模和叢集維運成本。

## 6. API 與事件契約

### 6.1 對外 HTTP API（第一版）

| 方法 | 路徑 | 用途 |
|---|---|---|
| `POST` | `/v1/entry-events` | 模擬使用者在指定站點進站，建立旅程並觸發推薦 |
| `GET` | `/v1/recommendations/latest?journey_id=...` | 取得最新推薦結果 |
| `POST` | `/v1/redemptions` | 建立兌換；請求需有 `Idempotency-Key` |
| `GET` | `/v1/redemptions/{redemption_id}` | 查詢兌換與核銷狀態 |
| `POST` | `/v1/merchant-verifications` | 模擬商家 POS 核銷 |
| `GET` | `/healthz` | health check |

HTTP 是 demo 與外部呼叫入口；服務內部需要同步協作時使用 gRPC。所有 API 的 request / response 由 `.proto` 產生，並輸出 OpenAPI 規格與 Swagger UI。

### 6.2 共用事件信封

每個 Kafka event 都使用相同欄位，payload 依 event type 擴充：

```json
{
  "event_id": "01J...",
  "event_type": "recommendation.created.v1",
  "schema_version": 1,
  "occurred_at": "2026-07-24T14:00:00Z",
  "producer": "recommendation-service",
  "user_id_hash": "sha256:...",
  "journey_id": "journey_...",
  "recommendation_id": "rec_...",
  "trace_id": "...",
  "causation_id": "event that caused this event",
  "payload": {}
}
```

`event_id` 用於去重；`causation_id` 用於建立事件因果鏈；`trace_id` 用於從商業事件跳回技術 trace。

### 6.3 Kafka topics

| Topic | Producer | Consumer | Key | 用途 |
|---|---|---|---|---|
| `journey.entered.v1` | gateway | recommendation | `user_id_hash` | 進站觸發 |
| `recommendation.created.v1` | recommendation | notification mock、analytics | `user_id_hash` | 推薦已產生 |
| `notification.sent.v1` | notification mock | analytics | `user_id_hash` | 推播送出 |
| `notification.delivered.v1` | notification mock | analytics | `user_id_hash` | 供應商／裝置確認送達 |
| `recommendation.impressed.v1` | demo client | analytics | `user_id_hash` | 使用者實際看到內容 |
| `recommendation.clicked.v1` | demo client | analytics | `user_id_hash` | 使用者開啟或點擊 |
| `redemption.succeeded.v1` | redemption | analytics | `user_id_hash` | 兌換交易成功 |
| `merchant.verified.v1` | merchant mock | analytics | `journey_id` | 商家核銷，最強轉換證據 |
| `visit.attributed.v1` | attribution worker / demo | analytics | `journey_id` | 推估到店，不能視為 POS 核銷 |
| `offer.changed.v1` | offer administration / seed importer | embedding indexer | `offer_id` | 更新可重建的優惠向量索引 |
| `dlq.v1` | 各 consumer | 維運人員 | 原 event key | 失敗事件保留與人工檢查 |

同一個 `user_id_hash` 會寫入同一 partition，確保同一使用者的進站、推薦與兌換順序在該 partition 內一致。不同 consumer group 可獨立消費相同事件：推薦、分析與可觀測性不互相阻塞。

### 6.4 Web Push provider 邊界

```mermaid
flowchart LR
  WebApp[Web app] -->|POST /v1/notification-subscriptions| Gateway[Gateway]
  Gateway --> PG[(PostgreSQL)]
  Recommendation[Recommendation service] -->|recommendation.created.v1| Kafka[(Kafka)]
  Kafka --> Worker[Notification worker]
  Worker -->|timezone + local window + active subscription| PG
  Worker --> Provider{Provider}
  Provider -->|mock| Mock[Deterministic mock]
  Provider -->|webpush + VAPID| PushService[Browser Push Service]
  Mock -->|notification.sent.v1 + delivered| Kafka
  PushService -->|notification.sent.v1| Kafka
  PushService --> Browser[Service worker]
  Kafka --> Analytics[Analytics consumer]
```

前端有 `VITE_VAPID_PUBLIC_KEY` 時會透過 `PushManager.subscribe()` 註冊真實 browser subscription，service worker 接收 push 並顯示通知；未設定時才使用 `https://demo.invalid/...` synthetic subscription。Backend 以 `NOTIFICATION_PROVIDER=webpush`、VAPID subject/public/private key 啟用加密發送，預設仍是 mock。撤銷會將 subscription 標記為 inactive；worker 不會把 raw device token 寫入事件、log 或 trace。Web Push provider 只能確認 push service 接受，因此 `notification.delivered.v1` 仍保留給 deterministic mock 或未來的 client receipt。

## 7. 資料設計

### 7.1 PostgreSQL：交易真實來源

| 資料表 | 目的 |
|---|---|
| `users` | demo 使用者、點數餘額摘要與偏好 reference |
| `points_ledger` | 不可變點數帳本，記錄增加、扣除與原因 |
| `offers` | 優惠內容、所屬站點／商家、有效期間、成本與標籤 |
| `inventory` | 每個優惠剩餘數量與版本號 |
| `recommendations` | 推薦結果、決策理由、候選與分數摘要 |
| `redemptions` | 兌換單、冪等 key、狀態與核銷資訊 |
| `outbox_events` | 與業務資料同 transaction 寫入、等待發布的事件 |
| `processed_events` | consumer 已處理事件 ID，用於冪等去重 |

### 7.2 Redis：可重建的暫存資料

推薦服務讀取下列 key；資料遺失後可由 seed data 或離峰預計算重新建立，因此不視為真實來源。

```text
preference:{user_id_hash}   # 預測目的地、偏好標籤、有效到期時間
dedupe:{event_id}           # 短期事件去重
rate_limit:{user_id_hash}   # 推播／推薦節流
```

### 7.3 ClickHouse：商業分析資料

`product_events` 是 append-only 事件表，保留原始產品行為；可依日期、站點、優惠、商家、experiment 與 variant 聚合。

建議的核心欄位：

```text
occurred_at, event_name, journey_id, recommendation_id,
user_id_hash, station_id, offer_id, merchant_id,
experiment_id, variant, attribution_type, trace_id
```

`attribution_type` 至少區分：

- `observed_pos_verified`：商家 POS mock／真實核銷確認，屬於事實。
- `inferred_visit`：以取得同意的訊號推估到店，屬於估計，不能和核銷混為同一 KPI。
- `none`：尚未有足夠證據歸因。

### 7.4 Qdrant：優惠語意索引

Qdrant collection `offer_embeddings_v1` 以穩定映射後的 point UUID 保存向量，原始 `offer_id` 留在 payload。payload 可包含 `station_ids`、`category`、`merchant_id`、`content_version` 與 `embedding_model`，用於縮小候選集。

不應將庫存、點數餘額或兌換資格當作 Qdrant 的可靠決策資料；這些資訊即使複製到 payload，也只能作為預篩選，最終必須回 PostgreSQL 驗證。

優惠內容更新時發布 `offer.changed.v1`，由 `embedding-indexer` 產生 embedding 後 upsert。換 embedding model 時建立新 collection，例如 `offer_embeddings_v2`，完整重建與驗證後再切換讀取設定。

#### 現在怎麼做

- `embedding-indexer` 啟動時建立 collection 與 station/category/version payload index，並 bootstrap PostgreSQL 中的 active offers。
- `offer.changed.v1` 的 UPSERT／DELETE 只有在 Qdrant 成功後才 commit Kafka offset。
- `demo-hash-v1` 以 SHA-256 將 canonical offer document 轉成固定 32 維向量；推薦 query 則以偏好 category、預算與 station context 產生同維度向量。
- 這個 hash **不是語意 embedding**，只用來驗證事件、重建索引、Qdrant filter/search、候選回查與失敗處理能端對端運作。

#### 語意 embedding adapter

常用站點、推播時段、點數預算與明確 category 是結構化條件，應保存在 PostgreSQL，並以 filter／rule score 使用；profile vector 只在進站查詢時即時產生，不建立持久 user vector，也不把敏感位置偏好寫進 Qdrant。

語意模型只負責把「優惠文件」與「當下查詢文件」投影到同一向量空間：

```text
offer document = title + description + normalized category + merchant type
query document = preferred categories + current/favorite station context + time context
```

```mermaid
flowchart LR
    OfferDB["PostgreSQL offers\nsource of truth"] -->|"offer.changed.v1"| Kafka["Kafka"]
    Kafka --> Indexer["embedding-indexer"]
    Indexer --> OfferDoc["Canonical offer document"]
    OfferDoc --> Embedder["同一個 multilingual Embedder"]
    Embedder -->|"offer vector + versioned payload"| Qdrant["Qdrant offer_embeddings_v2"]

    Pref["PostgreSQL user preferences\nstructured, not vector DB"] --> Query["Query document builder"]
    Context["station / time / journey context"] --> Query
    Query --> Embedder
    Embedder -->|"query vector + station filter"| Qdrant
    Qdrant -->|"top-K offer IDs only"| Validate["PostgreSQL validation"]
    Validate --> Rank["Rules: inventory / points / eligibility / preference"]
    Rank --> Result["Final recommendation + reasons"]
```

模型不綁定特定供應商；adapter 使用通用 `/v1/embeddings` contract。預設仍是 hash fallback，啟用語意模式時以同一個 adapter 產生 offer vector 與每次進站的 profile/query vector。選型條件是繁體中文／多語能力、可在 demo 硬體執行、批次索引成本與穩定向量維度。導入時會：

1. 準備一小組「偏好／站點情境 → 預期優惠」標註案例，先量測 Recall@K，而不是只憑模型名稱決定。
2. 以同一個 `Embedder` adapter 同時產生 offer 與 profile/query vectors，並正規化輸入文字；profile vector 目前在每次進站時即時產生，不另存敏感向量。
3. 建立新 collection `offer_embeddings_v2`，記錄 `embedding_model` 與 `content_version`，完整重建後和 v1 比較。
4. 驗證召回品質、延遲與 fallback 後再切換設定；保留 v1 以便回退，不在原 collection 原地混用不同模型。
5. Embedding service 不可用時，保留 `EMBEDDING_MODE=hash` fallback；核心推薦仍可運作，硬性站點／點數／庫存規則不交給模型決定。

## 8. 一致性與失敗處理

### 8.1 Transactional outbox

兌換成功時，不能先寫資料庫再直接送 Kafka，否則可能發生「DB 已扣點但 Kafka 發送失敗」。解法：在同一筆 PostgreSQL transaction 中完成：

1. 驗證 `Idempotency-Key`。
2. 鎖定點數與庫存資料。
3. 建立 `redemptions`、寫入 `points_ledger`、更新 `inventory`。
4. 寫入 `outbox_events`。
5. commit。
6. outbox publisher 非同步讀取未送出資料、發布到 Kafka，成功後標記為已送。

如此即使 Kafka 或 publisher 暫時失敗，事件仍留在資料庫中可重試；而 consumer 以 `event_id` 寫入 `processed_events` 達成 at-least-once 下的冪等處理。

### 8.2 重試與 DLQ

- 可恢復錯誤（網路、短暫 DB 失敗）採指數退避重試。
- 不可恢復錯誤（schema 不相容、缺少必要欄位）送往 `dlq.v1`。
- DLQ event 必須保留原 event、失敗原因、consumer 名稱與重試次數。
- demo 只需能觀察與手動重播；不必建置完整營運後台。

## 9. 推薦決策流程

```mermaid
sequenceDiagram
    participant C as Demo Client
    participant G as Gateway
    participant K as Kafka
    participant R as Recommendation
    participant Redis as Redis
    participant V as Qdrant
    participant PG as PostgreSQL
    participant L as LLM Adapter

    C->>G: POST /v1/entry-events
    G->>G: 建立 journey_id 與 trace context
    G->>K: journey.entered.v1
    K->>R: consume entry event
    R->>Redis: 讀取預算偏好與預測目的地
    R->>V: 語意檢索 top-K 候選 offer IDs
    R->>PG: 驗證有效優惠、即時庫存與點數資格
    R->>R: 規則過濾與排序，選定唯一 offer
    R->>L: 可選：傳入已驗證 facts 與嚴格 deadline
    L-->>R: 文案 JSON 或 timeout / error
    R->>PG: 保存 recommendation + explainability
    R->>K: recommendation.created.v1
    C->>G: GET latest recommendation
    G-->>C: 推薦、理由、兌換入口
```

### MVP 排序規則

推薦結果必須附帶可解釋理由。初版可採固定權重：

```text
final_score =
  0.35 * preference_match +
  0.25 * point_roi +
  0.20 * station_proximity +
  0.10 * time_context +
  0.10 * inventory_urgency
```

硬性過濾條件：優惠已過期、無庫存、點數不足、使用者不符合資格時，一律不可送入排序。

### 9.1 LLM 文案生成規範

LLM 若啟用，只能將「最終已選定優惠 + 可使用的結構化情境」轉成推薦文案。它使用輕量開源 instruct model，經 `CopyGenerator` adapter 呼叫；模型可替換，業務服務不得依賴特定供應商。

輸入只提供結構化 facts，例如站點、已選優惠名稱、已驗證折抵、點數、情境與語氣。輸出必須符合固定 JSON schema：`title`、`body`、`tone`。輸出長度受限，且不得新增未提供的商家、價格、點數、有效期或資格資訊。

推薦服務必須先生成確定可用的模板文案。LLM 僅能在嚴格 deadline 內嘗試改寫；超時、錯誤、JSON 不合法、內容違反 facts 或安全規則時，一律回退為模板，不能阻斷兩秒 SLA 或改變推薦結果。

LLM 不負責：候選召回、最終排序、目的地預測、庫存／點數驗證、扣點、交易狀態或商業 KPI 判定。

## 10. 可觀測性與商業分析

### 10.1 技術可觀測性：OpenTelemetry + LGTM

每個 HTTP request、gRPC call、Kafka producer 與 consumer 都傳遞 trace context。結構化 log 至少包含：

```text
timestamp, level, service_name, trace_id, span_id,
journey_id, recommendation_id, event_id, message, error_code
```

監控指標：

- **RED**：request rate、error rate、duration（P50 / P95 / P99）。
- **Kafka**：consumer lag、重試數、DLQ 數、publish / consume 延遲。
- **推薦**：端到端延遲、Redis 命中率、候選數、各規則淘汰數、LLM fallback 次數。
- **交易**：兌換成功／失敗率、點數不足、庫存不足、冪等命中數、outbox backlog。

本機先使用單一 `grafana/otel-lgtm` 開發容器；未來可拆為 OTel Collector、Loki、Tempo、Mimir 與 Grafana。

### 10.2 LLM 可觀測性

若啟用 LLM adapter，記錄：

- model 名稱、prompt template version、temperature。
- input / output token 數、延遲、估計成本。
- retriever candidate IDs、最終選取優惠、schema validation 與 fallback 原因。
- 安全過濾結果與錯誤碼。

不得記錄完整 prompt、使用者原始身分、敏感位置歷史或 API key。

### 10.3 商業 KPI 與歸因

| 層級 | 事件 | 定義 |
|---|---|---|
| 投遞 | `notification.sent` / `notification.delivered` | 推播已送出／供應商或裝置確認送達 |
| 曝光 | `recommendation.impressed` | 使用者實際看到推薦內容 |
| 互動 | `recommendation.clicked` | 使用者點擊或開啟兌換頁 |
| 交易 | `redemption.succeeded` | 點數扣除與兌換單建立成功 |
| 到店 | `merchant.verified` | 商家核銷確認；最強轉換證據 |

為了區分相關性與因果效果，產品事件必須攜帶 `experiment_id` 與 `variant`。例如保留 10% 符合資格者為 holdout group、不推播，再比較兩組核銷率；這才能估算推播帶來的增量，而非只報告「收到推播的人比較常兌換」。

## 11. 本機部署與設定

Docker Compose 應包含：

```text
gateway-service
recommendation-service
redemption-service
analytics-consumer
kafka (KRaft, single broker)
kafka-init (deterministic topic bootstrap)
postgres
redis
clickhouse
qdrant
otel-lgtm
```

環境設定由 `.env.example` 描述。任何秘密資訊都不能進入 Git；MVP 預設使用 mock weather、mock notification、mock merchant verification，不需第三方憑證。

LLM endpoint 也必須由設定注入。基礎 Compose 預設以 template / mock CopyGenerator 跑完整流程，不強制下載或啟動大型模型；需要展示 AI 時再指向本機輕量開源模型或相容 API。這可確保硬體條件不足、模型逾時或模型服務故障時，核心 demo 仍可運作。

### 11.1 SCRUM-36 壓測命令與證據

使用 Python 標準函式庫執行進站到推薦的並行 smoke load test，不需要額外壓測套件：

```bash
python3 scripts/load-test.py \
  --base-url http://127.0.0.1:8000 \
  --qdrant-url http://127.0.0.1:6333 \
  --requests 10 --concurrency 2 \
  --output /tmp/spacetime-load-test.json
```

先讓 Compose warm up 並確認 consumer group 已取得 partitions，再執行命令。輸出會保存 end-to-end P50/P95、recommendation decision latency、throughput、Qdrant search P50/P95、template fallback rate 與失敗數。Kafka lag 由同一次測試後執行 `kafka-consumer-groups.sh --describe` 保存；腳本不假裝從 API 推導 broker lag。Local template mode 的 `fallback_rate` 是預期值，切換 provider 後可用同一命令比較。一次可重現結果保留在 `docs/performance-scrum36.json`。

## 12. 開發順序

對應 Jira 的建議執行順序：

1. [SCRUM-6](https://hajimi-o.atlassian.net/browse/SCRUM-6)：Kratos mono-repo 與 Compose。
2. [SCRUM-7](https://hajimi-o.atlassian.net/browse/SCRUM-7)：Proto、OpenAPI、topic 與 schema。
3. [SCRUM-8](https://hajimi-o.atlassian.net/browse/SCRUM-8)：交易模型、migration、seed、outbox。
4. [SCRUM-9](https://hajimi-o.atlassian.net/browse/SCRUM-9)：進站到推薦。
5. [SCRUM-10](https://hajimi-o.atlassian.net/browse/SCRUM-10)：兌換到核銷模擬。
6. [SCRUM-11](https://hajimi-o.atlassian.net/browse/SCRUM-11)：ClickHouse 漏斗與商業 dashboard。
7. [SCRUM-12](https://hajimi-o.atlassian.net/browse/SCRUM-12)：OTel 與 LGTM。
8. [SCRUM-13](https://hajimi-o.atlassian.net/browse/SCRUM-13)：端對端 demo、測試、壓測與交付。

## 13. Demo 劇本

1. 執行 `docker compose up`，確認 services、Kafka、資料庫與 Grafana 健康。
2. 使用 demo user 在「信義安和站」送出進站事件。
3. 顯示 gateway 回傳的 `journey_id`，並在 Swagger UI／CLI 查詢推薦。
4. 顯示推薦理由：使用者偏好、時段、站點、向量候選與點數 ROI；同時展示模板文案或通過 schema 驗證的 LLM 文案。
5. 建立 redemption，展示點數帳本、庫存與 outbox event。
6. 透過 merchant mock 核銷。
7. 開啟 Grafana：
   - ClickHouse dashboard 顯示旅程與漏斗。
   - Tempo trace 顯示跨 gateway、Kafka、recommendation、redemption 的處理鏈。
   - metrics 顯示推薦延遲、consumer lag 與錯誤率。

## 14. MVP 驗收清單

- [x] 任一開發者可依 README 成功啟動環境。
- [x] API 文件可從 Swagger UI 瀏覽與操作。
- [x] Gateway 將進站 journey 與 `journey.entered.v1` outbox event 以同一筆 PostgreSQL transaction 建立；recommendation-service 透過 Kafka consumer 觸發推薦。
- [x] 推薦有可讀理由且不推薦過期、無庫存或點數不足優惠。
- [x] Qdrant 僅負責候選召回；最終優惠均已回 PostgreSQL 驗證，API/demo 會展示 `candidates[]`。
- [x] LLM 只能根據已驗證 facts 生成 JSON 文案，失敗時自動使用模板且不影響推薦結果。
- [x] 兌換交易可防止重複扣點，並使用 transactional outbox 發布事件。
- [x] Web Push 可註冊／撤銷訂閱，依使用者 timezone／local window 篩選；mock 產生 sent/delivered，VAPID provider 發送真實瀏覽器通知並產生 sent event。
- [x] ClickHouse 可查到完整漏斗事件；Grafana 可顯示核心商業 KPI。
- [x] Tempo trace 可連結關鍵服務與 Kafka 處理；logs 不含 PII。
- [x] SCRUM-36 壓測腳本與結果可重現；warm local Compose 的 E2E P95 335ms、Qdrant P95 11.69ms、Kafka lag 0。
- [x] Demo 預設不依賴真實北捷、POS、VAPID push 或 LLM API；需要真實 Web Push 時以環境變數啟用。

## 15. 後續演進方向

- 將 [SCRUM-38](https://hajimi-o.atlassian.net/browse/SCRUM-38) resolver 接到 Gateway 的可選 Beacon observation path；正式 provider 需先確認外部契約與 credentials。
- 將 user preference API 接上正式身份驗證與 Web app session；目前仍以 `user_id_hash` 作為 demo identity。
- 以標註案例評估 multilingual embedding，透過 `offer_embeddings_v2` 重建與切換，不在 v1 混用模型。
- 使用 CWA 天氣 API 替換 mock context provider。
- 接上真實推播與 POS adapter；保持 event schema 不變。
- 將 analytics consumer 擴展為 A/B test、商家報表與成本／ROI 模型。
- 將單機 Compose 演進至 Kubernetes、Kafka 多 broker、資料庫備援與正式 LGTM 部署。
