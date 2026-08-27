# Talos

Declarative [Talos Linux](https://www.talos.dev) machine configuration for this cluster, built
**toolless** from composable multi-document patches — native `talosctl machineconfig patch` +
[`sops`](https://github.com/getsops/sops) + [`minijinja-cli`](https://github.com/mitsuhiko/minijinja).
No `talhelper` / `talstomize` / `topf`. Nothing here is applied automatically; configs are rendered
on demand and pushed to nodes with `talosctl` via the [`task talos:*`](../.taskfiles/talos/Taskfile.yaml)
recipes.

> [!IMPORTANT]
> **Why toolless?** A third-party generator (like talhelper) has to add explicit support for every
> new Talos config field and format before you can use it. Rendering with plain `talosctl` means the
> config format is whatever the pinned `talosctl` understands — so **prerelease / release-candidate
> Talos always works day one**. This tree uses the Talos **1.14 typed multi-document** format.

> [!WARNING]
> This layout was **authored without ever rendering, dry-running, or applying it** (by request — no
> cluster or secret access during authoring). Treat it as untested. **Render every node and diff
> against the live config before applying** (see [Verifying](#verifying)).

## Layout

| Path                                       | Purpose                                                                    |
| ------------------------------------------ | -------------------------------------------------------------------------- |
| `base.yaml`                                | Documents applied to **every** node (typed multi-doc). Injects `schematic`. |
| `controlplane.yaml`                        | Control-plane-only documents, including `machine.type: controlplane`.       |
| `worker.yaml`                              | Worker-only documents (`machine.type: worker`).                             |
| `nodes/<role>/<node>.yaml`                 | Per-node docs: hostname, static address, install disk, labels/taints, NIC.  |
| `nodes/<role>/<node>.schematic.yaml`       | Optional per-node [Image Factory](https://factory.talos.dev) override.      |
| `schematic.yaml`                           | Shared Image Factory schematic (extensions + kernel args).                  |
| `hack/secrets.*.yq`                        | `yq` key-path maps: sops bundle → Talos secret docs (**no secret values**). |
| `talsecret.sops.yaml`                      | Encrypted secrets bundle (certs/keys/tokens). **Stays encrypted.**          |
| `talenv.sops.yaml`                         | Encrypted NUT/UPS credentials for the `nut-client` extension.               |
| `clusterconfig/`                           | Generated client `talosconfig` (gitignored).                                |

**Role is decided by directory placement.** A node in `nodes/controlplane/` gets `controlplane.yaml`;
a node in `nodes/worker/` gets `worker.yaml`. `machine.type` comes from the role patch, never the node
file.

## Rendering

`task talos:render-config NODE=<node>` builds the final machine config in layered patches (later
patches strategically merge into earlier ones — same `kind`/`name` deep-merges, new docs append):

```
talosctl machineconfig patch base.yaml \
    -p @controlplane.yaml|worker.yaml \
    -p @nodes/<role>/<node>.yaml \
    -p @<sops -d talsecret.sops.yaml | yq --from-file hack/secrets.all.yq> \
    -p @<sops -d talenv.sops.yaml    | yq --from-file hack/secrets.nut.yq> \
    -p @<sops -d talsecret.sops.yaml | yq --from-file hack/secrets.controlplane.yq>   # control plane only
```

- `base.yaml` passes through `minijinja-cli`, which injects the Image Factory **schematic id** (POSTed
  to the factory at render time) into the `UnattendedInstallConfig` installer image.
- **Secrets never live in git and never touch `base.yaml`.** They are decrypted from the two sops
  bundles at render time and mapped into Talos secret documents by the committed `hack/secrets.*.yq`
  key-path expressions (which contain only field mappings, zero secret values).
- Because `go-task`'s shell has no process substitution, `render-config` writes the patch layers to a
  `umask 077` tmpdir that is wiped on exit, rather than using `<(...)`.

## Secret split (and the CA-unit gotcha)

`machine.ca`, `cluster.ca`, and `cluster.etcd.ca` merge as a **cert+key unit** — a patch that supplies
only `key` would blank `crt`. So:

- `hack/secrets.all.yq` (all nodes) emits **crt only** for the CAs, plus the join/bootstrap tokens and
  the `DiscoveryIdentityConfig`. Workers get exactly this — never a private key.
- `hack/secrets.controlplane.yq` (control plane only) emits the CA **crt + key together** (the unit),
  the service-account key, the etcd CA, and the etcd secretbox encryption key
  (`KubeEtcdEncryptionConfig`). Applied last so its complete `ca` unit wins.

## Schematics

The schematic defines the Image Factory build (system extensions + kernel args). Resolution is
per node: `nodes/<role>/<node>.schematic.yaml` wins when present, otherwise the shared
`schematic.yaml` applies (`talos-01`/`talos-02` use the shared one; `talos-03` drops zfs; `talos-04`
adds realtek + the Strix Halo/ROCm kernel args and drops zfs). Overrides are **complete files**, not
deltas.

## Commands

```sh
task talos:render-config NODE=talos-01     # render a node's full machine config to stdout
task talos:apply-node    NODE=talos-01     # render and apply (talosctl apply-config); IP auto-derived
task talos:schematic-id  [NODE=talos-04]   # POST a schematic to the factory, print its content id
task talos:download-image VERSION=v1.14.0 [NODE=talos-04]   # fetch a metal ISO from the factory
task talos:talosconfig                     # (re)generate the client talosconfig from the sops bundle
task talos:reboot-node   IP=192.168.10.10  # reboot / shutdown / reset wrappers
task talos:shutdown-node IP=192.168.10.10
task talos:reset-node    IP=192.168.10.10  # wipe back to maintenance mode
```

Talos/Kubernetes **upgrades run in-cluster via [tuppr](../kubernetes/apps/system-upgrade/tuppr)**;
`task talos:upgrade-node` / `upgrade-k8s` are manual break-glass wrappers only.

## Verifying

Because this was authored untested, verify before trusting it:

```sh
# 1. Does each node render at all?
task talos:render-config NODE=talos-01

# 2. Diff the rendered config against what's live on the node (READ-ONLY dry run)
task talos:render-config NODE=talos-01 | talosctl -n 192.168.10.10 apply-config -f /dev/stdin --dry-run
```

`--dry-run` should report the diff without changing anything; expect "No changes." once the migration
is faithful. Only then `task talos:apply-node`.
