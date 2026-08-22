# PostgreSQL bootstrap

The official PostgreSQL image runs these ordered SQL files only when its data volume is empty. They form the reproducible MVP baseline: schema first, demo data second, then additive recommendation metadata.

To recreate local demo data, explicitly run `docker compose --env-file .env -f deploy/compose/compose.yaml down --volumes`, then start Compose again. This deletes all local service data.

When a non-initial schema change is needed, add a new numbered SQL migration and do not edit an already-applied file. `000003_recommendation_copy_source.sql` adds the copy source returned by the recommendation API, `000004_recommendation_candidate_summary.sql` stores candidate scores and explainability, `000005_recommendation_latency.sql` stores decision latency for the latest-recommendation query, `000006_user_preferences.sql` adds the App-facing profile and preference fields, `000007_notification_subscriptions.sql` adds Web Push subscriptions and delivery dedupe records, `000008_offer_category.sql` adds normalized offer categories, `000009_user_preference_weights.sql` stores bounded category feedback weights, `000010_demo_station_catalog.sql` keeps the selectable demo station and offer catalog consistent on existing volumes, `000011_demo_station_catalog_expansion.sql` adds the larger multi-station demo catalog, and `000012_beacon_station_catalog.sql` adds the full named Beacon station catalog and baseline offers.

The official PostgreSQL image does not apply new files to an existing data volume. After Docker Compose is running, apply the catalog migrations and refresh the embedding index with:

```bash
make migrate
```

Use `ENV_FILE=/path/to/.env make migrate` when the environment file is elsewhere. The target is safe to repeat: the catalog SQL uses upserts, and the indexer is restarted after the database update so the new offers are embedded into Qdrant.
