# Cluster Bootstrap & Restore Runbook

## Prerequisites

The age private key is the only secret that lives outside of git. Store it somewhere safe (e.g. a Bitwarden secure note).

## Fresh Cluster Bootstrap

This is the only manual step required:

```bash
kubectl create namespace flux-system

kubectl create secret generic sops-age \
  --namespace=flux-system \
  --from-literal=age.agekey=<your-age-private-key>
```

Then kick off Flux bootstrap as normal. Everything else is automatic:

1. Flux decrypts SOPS secrets → S3 credentials and Infisical `universal-auth-credentials` land
2. CNPG restores postgres from S3 → Infisical comes up with data intact
3. ESO `ClusterSecretStore` becomes healthy → all other secrets sync
4. Remaining apps come up

ESO will log sync failures while postgres is restoring — this is expected and will self-resolve as Infisical becomes healthy.

---

## Postgres Restore Procedure

### 1. Update the cluster manifest

In `kubernetes/components/database/postgres/cluster.yaml`:

- Uncomment `bootstrap.recovery`
- Bump `serverName` in `plugins` to the next version (e.g. `postgres-v2` → `postgres-v3`)
- Set `externalClusters[].serverName` to the **previous** `serverName` value

```yaml
bootstrap:
    recovery:
        source: source
plugins:
    - name: &plugin barman-cloud.cloudnative-pg.io
      isWALArchiver: true
      parameters:
          barmanObjectName: &store minio-store
          serverName: postgres-v3 # bumped from v2
externalClusters:
    - name: source
      plugin:
          name: *plugin
          parameters:
              barmanObjectName: *store
              serverName: postgres-v2 # was the previous serverName
```

### 2. Commit and apply

```bash
git add kubernetes/components/database/postgres/cluster.yaml
git commit -m "chore: restore postgres from S3 backup"
git push
```

Flux will apply the manifest. Monitor progress:

```bash
kubectl get cluster postgres -n database -w

# or for more detail
kubectl describe cluster postgres -n database
```

### 3. Verify restore completed

```bash
kubectl get cluster postgres -n database
# STATUS should show: Cluster in healthy state
```

Check Infisical is up and ESO is syncing:

```bash
kubectl get clustersecretstore infisical
# STATUS should show: Valid
```

### 4. Clean up bootstrap section

Once the cluster is `Ready`, remove the `bootstrap.recovery` block and commit:

```yaml
# bootstrap:
#   recovery:
#     source: source
plugins:
    - name: &plugin barman-cloud.cloudnative-pg.io
      isWALArchiver: true
      parameters:
          barmanObjectName: &store minio-store
          serverName: postgres-v3 # keep this — it's the current version
externalClusters:
    - name: source
      plugin:
          name: *plugin
          parameters:
              barmanObjectName: *store
              serverName: postgres-v2 # keep this — needed for next restore
```

```bash
git add kubernetes/components/database/postgres/cluster.yaml
git commit -m "chore: remove bootstrap.recovery after successful restore"
git push
```

---

## Version History

Track serverName bumps here so the next restore is always unambiguous.

| Date    | Event           | serverName |
| ------- | --------------- | ---------- |
| initial | cluster created | `postgres` |
