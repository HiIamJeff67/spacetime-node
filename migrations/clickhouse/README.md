# ClickHouse migrations

Docker Compose mounts this directory into ClickHouse's initialization directory. The scripts run only when the `clickhouse-data` volume is first created.

For a local demo reset after adding a migration, first stop the stack, then explicitly remove the project ClickHouse volume and start Compose again. Do not use that reset for data you need to keep.
