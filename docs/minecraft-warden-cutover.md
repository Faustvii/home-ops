# Minecraft Warden cutover — 2026-10-31

The live cutover uses the existing PVCs. Fog already used Warden and stays unchanged.

| MinecraftServer | Existing PVC | Version | World |
| --- | --- | --- | --- |
| main | minecraft | 1.21.11 | Wonderland |
| chunklock | minecraft-chunklock | 1.21.11 | Chunklock |
| mindscraft-1-21-4 | minecraft-1-21-4 | 1.21.4 | world |

The Warden router uses `192.168.10.30`, with main as its fallback. All Minecraft and voice connections use the router. The direct main address is removed. BlueMap uses the `main` Service on port 8100.

The old Helm releases were stopped and uninstalled. All three PVCs have successful manual Kopiur snapshots with `pin: true` and `deletionPolicy: Retain`:

- `minecraft-pre-warden-20261031`
- `minecraft-chunklock-pre-warden-20261031`
- `minecraft-1-21-4-pre-warden-20261031`

Chunklock uses a 6Gi heap inside its existing 8Gi container limit. Warden sets the initial heap equal to the maximum; the previous 8Gi heap caused an OOM at startup.

Flux remains suspended until the owner pushes the migration commit. `flux-system/cluster-apps` is also suspended so it cannot recreate the old child definitions. Other existing child Kustomizations continue to reconcile.

After pushing this commit to main:

```sh
flux reconcile source git flux-system -n flux-system
# Confirm the fetched revision includes the migration commit.
flux resume kustomization warden-router -n games
flux reconcile kustomization warden-router -n games
flux resume kustomization cluster-apps -n flux-system
flux reconcile kustomization cluster-apps -n flux-system
for name in minecraft minecraft-chunklock minecraft-1-21-4; do
  flux resume kustomization "$name" -n games
  flux reconcile kustomization "$name" -n games
done
```

The old `mc-router` Kustomization stays suspended until the parent prunes it. The retained server Kustomization names preserve PVC and Kopiur identities. Pruning removes the old exporter monitors, dashboard, datapack ConfigMaps, restart resources, and chart sources. The observability Kustomization removes the obsolete silence after the push.

The owner must join all three servers through the router and confirm the expected worlds and voice chat. Check `status.lastStart` on each MinecraftServer. Keep the pinned snapshots until those checks pass.

For rollback, stop the Warden servers with `spec.suspend: true` and wait for all three pods to disappear. Restore the previous Git manifests with reconciliation still suspended. Never run both controllers' server pods against one PVC. If world data needs rollback, restore the corresponding pinned Kopiur snapshot before starting a legacy server.
