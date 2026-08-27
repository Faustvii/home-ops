# Talos configuration (talstomize)

This directory holds the Talos Linux machine configuration for the cluster, managed with
[talstomize](https://github.com/mirceanton/talstomize) (this repo migrated off the
deprecated `talhelper`).

> [!IMPORTANT]
> This migration was **authored only** — nothing here has been built, rendered,
> dry-run, or applied against the live cluster. `talstomize build` submits schematics to
> the Talos Image Factory and decrypts the secrets bundle, so verify the render in a
> throwaway environment before you trust it. Treat the whole layout as **untested**.

## Layout

| File / dir | Purpose |
| --- | --- |
| `talstomize.yaml` | The single manifest: cluster name/endpoint, `kubernetesVersion`, secrets ref, per-node ip/kind/patches, and per-node (or cluster-default) `installer.schematic`. Also carries the `talosVersion` (on `installer.talosVersion`) with the Renovate comment. |
| `talsecret.sops.yaml` | sops-encrypted secrets bundle (`talosctl gen secrets` layout). talstomize auto-detects the top-level `sops:` key and decrypts via `sops` at build time. **Never** committed decrypted. |
| `talenv.sops.yaml` | sops-encrypted **env vars** (UPS-monitor creds) consumed by `patches/global/machine-nut.yaml` via `${upsmonHost}` / `${upsmonUser}` / `${upsmonPass}`. Injected at build time with `sops exec-env` (see the Taskfiles). Not versions — kept, not retired. |
| `patches/` | Strategic-merge patches referenced by path from the manifest — see `patches/README.md`. |
| `clusterconfig/` | Client `talosconfig` lives here (gitignored). talstomize does not regenerate it. |
| `_out/` | `talstomize build` output — **gitignored**, never committed. |

Versions (`talosVersion`, `kubernetesVersion`) moved from the retired `talenv.yaml`
directly into `talstomize.yaml` with their `# renovate:` comments (Renovate only scans
`talos/*.yaml`, so a YAML file — not a `.env` — keeps tracking working). Talos/K8s
**upgrades are owned by tuppr in-cluster**; talstomize deliberately has no upgrade command.

## Commands

All commands run from this directory (or via the wrapper `task` targets, which also inject
the UPS creds with `sops exec-env`).

```sh
# Render every node's machine config to ./_out (nothing is applied)
talstomize build .
# → task talos:generate-config

# Diff the rendered config against the live cluster (all nodes, or one)
talstomize diff -f .
talstomize diff -f . --node talos-01
# → task talos:diff [NODE=talos-01]

# Apply to a single node (name = the key under `nodes:` in talstomize.yaml)
talstomize apply -f . --node talos-01 -- --mode=auto
# → task talos:apply-node NODE=talos-01 MODE=auto

# Flags after `--` are passed straight through to talosctl, e.g. first install:
talstomize apply -f . --node talos-01 -- --insecure
```

`talstomize apply` / `diff` are thin wrappers around `talosctl` and need a valid
`talosconfig` (see `TALOSCONFIG` in `.mise.toml`). They never run `upgrade` /
`upgrade-k8s` — that's tuppr's job.
