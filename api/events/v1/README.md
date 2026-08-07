# Event contract v1

`events.schema.json` is the JSON Schema source for every Kafka message in this MVP. The Kafka record value is the envelope; record headers are transport metadata only and are not part of the business contract.

| Topic | Producer | Consumer | Record key |
| --- | --- | --- | --- |
| `journey.entered.v1` | gateway-service | recommendation-service, analytics-consumer | `user_id_hash` |
| `recommendation.created.v1` | recommendation-service | analytics-consumer | `user_id_hash` |
| `notification.sent.v1` | notification mock | analytics-consumer | `user_id_hash` |
| `notification.delivered.v1` | notification mock | analytics-consumer | `user_id_hash` |
| `recommendation.impressed.v1` | demo client | analytics-consumer | `user_id_hash` |
| `recommendation.clicked.v1` | demo client | analytics-consumer | `user_id_hash` |
| `redemption.succeeded.v1` | redemption-service outbox | analytics-consumer | `user_id_hash` |
| `merchant.verified.v1` | merchant verification mock | analytics-consumer | `journey_id` |
| `offer.changed.v1` | offer administration / seed importer | embedding indexer | `offer_id` |
| `dlq.v1` | failed consumer | operator replay tool | original record key |

All records include `event_id`, `event_type`, `schema_version`, `occurred_at`, `producer`, and `trace_id`. `causation_id`, `journey_id`, `recommendation_id`, and `user_id_hash` are included when the event has that context; raw user identifiers are forbidden. Notification and engagement events must include `journey_id`, `recommendation_id`, and `user_id_hash`, which lets analytics attribute exposure, click, redemption, and merchant verification to the same recommendation.

Compatibility rules: add only optional fields in v1, never reuse or change a field's meaning or type, and let consumers ignore unknown payload fields. Any breaking change requires a new topic suffix such as `.v2`; its producer and consumer run alongside v1 until replay and migration are complete.

`offer.changed.v1` may include `title`, `description`, `station_id`, and `category` so the indexer can update Qdrant without a second read. They remain optional for compatibility; when omitted, the indexer loads the current offer from PostgreSQL.
