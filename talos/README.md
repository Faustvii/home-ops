# Talos configuration (managed with topf)

This directory holds the Talos Linux configuration for the cluster. It is managed
with [topf](https://github.com/postfinance/topf) (migrated from talhelper).

> [!IMPORTANT]
> This layout was authored as a migration draft and has **not** been rendered,
> dry-run, or applied against the cluster. Verify it (`task talos:render`, inspect
> the output, then a manual `topf apply --dry-run`) before applying anything.

## Layout

| Path | Purpose |
| ---- | ------- |
| `topf.yaml` | Cluster meta (name, endpoint, versions), node list, schematic + secrets references, template `data`. |
| `secrets.sops.yaml` | sops-encrypted Talos secret bundle (certs/keys/tokens). Referenced by `secretsPath`. Never decrypt into git. |
| `data.sops.yaml` | sops-encrypted template values (upsmon credentials) resolved by `data:` in `topf.yaml`. |
| `schematic.yaml` | Base image-factory schematic (talos-01 / talos-02). |
| `talos-03.schematic.yaml`, `talos-04.schematic.yaml` | Per-node schematic overrides (`schematicId: "@..."`). |
| `all/` | Patches applied to every node. |
| `control-plane/` | Patches applied to control-plane nodes only. |
| `node/<host>/` | Patches applied to a single node (install disk, network, kernel modules, volumes, taints). |
| `clusterconfig/` | Render output + generated talosconfig. **gitignored** — decrypted, never commit. |

Patches merge in order: `all/` → `control-plane/` (role) → `node/<host>/`, lexicographically
within each folder. `.yaml`/`.yml` are strategic-merge patches; `.yaml.tpl` are Go-templated.

## Common commands

All commands assume `mise`/`task` set `TALOSCONFIG`, `KUBECONFIG`, and `SOPS_AGE_KEY_FILE`.

```sh
# Render every node's final machine config to talos/clusterconfig (safe, offline)
task talos:render

# Print the image-factory schematic IDs (optionally one node)
task talos:schematic-ids
task talos:schematic-ids NODE=talos-04

# Generate the client talosconfig into $TALOSCONFIG
task talos:talosconfig

# Generate an admin kubeconfig (24h) into $KUBECONFIG
task talos:kubeconfig

# Apply config to a single node (NODE = hostname; MODE defaults to auto)
task talos:apply-node NODE=talos-01
task talos:apply-node NODE=talos-01 MODE=no-reboot

# Reset nodes back to maintenance mode (destructive, prompts)
task talos:reset
```

Raw topf equivalents (run from this directory):

```sh
topf --topfconfig topf.yaml render --output clusterconfig   # render all nodes
topf --topfconfig topf.yaml apply --dry-run                 # preview diff, applies nothing
topf --topfconfig topf.yaml apply --nodes-filter '^talos-01$'
topf --topfconfig topf.yaml schematic-ids
```

## Bootstrapping a fresh cluster

`task bootstrap:talos` generates a secret bundle if missing, writes talosconfig,
then `topf apply --auto-bootstrap` (applies to maintenance-mode nodes and bootstraps
etcd), and finally writes a kubeconfig.

## Upgrades

Talos and Kubernetes **upgrades are handled in-cluster by tuppr**
(`kubernetes/apps/system-upgrade/tuppr`), not by topf. Keep `talosVersion` /
`kubernetesVersion` in `topf.yaml` in sync with what tuppr rolls out.

## Secrets

- `secrets.sops.yaml` and `data.sops.yaml` are sops-encrypted (age). topf decrypts
  them internally on read; rendered/applied configs contain the plaintext only in
  memory (and redacted from stdout by default).
- The `.sops.yaml` creation rule `talos/.*\.sops\.ya?ml` keeps any `*.sops.yaml`
  here encrypt-on-write. Do not commit decrypted machine configs — `clusterconfig/`
  is gitignored for exactly this reason.
