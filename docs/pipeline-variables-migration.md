# Pipeline variables migration

Earlier versions exported pipeline variables as `KEY:VALUE` pairs in the `variables` label. The default filter was `.*`, so enabling the feature exported every variable value, including credentials. Those values were also persisted in Redis.

## New behavior

- Only variable **keys** are exported. Values are discarded as soon as the GitLab API response is read. They never reach Redis, Prometheus labels, logs, or traces.
- Selection is default-deny. A key is exported only when it is listed in `pull.pipeline.variables.keys` or matches a non-empty `pull.pipeline.variables.regexp`.
- If neither is configured, the exporter does not call the GitLab pipeline variables API.

```yaml
project_defaults:
  pull:
    pipeline:
      variables:
        enabled: true
        keys: [DEPLOY_ENV, RELEASE_CHANNEL]
```

The label then looks like `variables="DEPLOY_ENV,RELEASE_CHANNEL"`. Keys are sorted, de-duplicated, and comma-separated.

## Configuration changes

| Before | After |
| --- | --- |
| `regexp` defaults to `.*` (everything) | `regexp` defaults to `""` (nothing) |
| No explicit allowlist | New `keys` list of exact key names |
| Label value `KEY:VALUE,...` | Label value `KEY,...` |

If you relied on the old default, add an explicit `keys` list. Dashboards and alerts that matched on values in the `variables` label must be updated, because values are no longer available.

## Automatic Redis cleanup

When a Redis URL is configured, the exporter purges legacy data once at startup, before it serves any metrics:

| Redis key | Action |
| --- | --- |
| `pipelines_variable` | Deleted. This legacy hash held raw `KEY:VALUE` strings. Keys are now stored in `pipeline_variable_keys`. |
| `pipelines` | Deleted. This cache may embed variable values and is rebuilt on the next pull. |
| `refs` | The variables stored on each ref's latest pipeline are cleared. |
| `metrics` | Every entry with a non-empty `variables` label is deleted. Those metrics are recreated on the next pull. |
| `migrations:pipeline_variable_keys_only` | Set as a marker, so the cleanup runs only once. |

The exporter refuses to start if this cleanup fails, so it never serves metrics from a database that may still contain values.

### Rolling upgrades

Stop every exporter replica running an older version before you start the new version. An older replica that keeps running writes values back after the cleanup has run. If that happened, stop the older replicas, delete the marker, and restart the new version:

```sh
redis-cli DEL migrations:pipeline_variable_keys_only
```

## Manual Redis cleanup

Use these commands when you cannot run the new version against the database, for example before decommissioning a Redis instance or restoring a backup.

Delete the hashes that hold or embed values:

```sh
redis-cli DEL pipelines_variable pipelines
```

The `refs` and `metrics` hashes may also contain values. The exporter rebuilds them, so you can delete them as well:

```sh
redis-cli DEL refs metrics
```

If you use a non-default Redis database number, add `-n <db>` to each command. Redis backups (RDB or AOF files) taken before the cleanup still contain the values and must be deleted or kept under the same protection as the secrets themselves.

## Data outside Redis

Values that were already scraped stay in Prometheus, in remote-write targets, and in their backups. Remove them separately, for example with the Prometheus TSDB admin API (requires `--web.enable-admin-api`):

```sh
curl -X POST -g 'http://prometheus:9090/api/v1/admin/tsdb/delete_series?match[]={variables=~".+:.*"}'
curl -X POST 'http://prometheus:9090/api/v1/admin/tsdb/clean_tombstones'
```

## Rotate exposed credentials

Rotate every credential that may have been exposed through the `variables` label or Redis. Deleting the data does not make a leaked secret safe again.
