# Leaderboard preview environments

Each pull request on `faustvii/beth-leaderboard` that has the `preview` label
gets its own live deployment. When the PR closes, Flux deletes the preview.

## How it works

1. CI pushes `ghcr.io/faustvii/beth-leaderboard:<head-sha>` and adds the
   `preview` label to the PR.
2. The `pull_request` webhook starts a reconcile of the
   `ResourceSetInputProvider`. The provider exports one input for each labeled
   PR.
3. The `ResourceSet` renders one `OCIRepository` and one `HelmRelease`
   (`pr-<id>-preview`) for each input, in the namespace `leaderboard-previews`.
4. CI reports the deployment status and preview link through GitHub deployments.

## Layout

```
kubernetes/apps/leaderboard/preview/           # namespace: leaderboard
├── ks.yaml
└── app/
    ├── rsip.yaml            # PR provider, filtered on the `preview` label
    ├── rset.yaml            # per-PR OCIRepository + HelmRelease template
    ├── webhook.yaml         # webhook token + Receiver
    ├── externalsecret.yaml  # GitHub App credentials
    └── rbac.yaml            # SA/preview-applier

kubernetes/apps/leaderboard-previews/          # namespace: leaderboard-previews
├── namespace.yaml           # Pod Security level `restricted`
└── runtime/app/
    ├── externalsecret.yaml  # Azure credentials for the previews
    ├── limitrange.yaml      # 1 CPU and 512Mi for each container at most
    ├── networkpolicy.yaml   # CiliumNetworkPolicy
    └── rbac.yaml            # Roles for preview-applier and preview-installer
```

The GitHub App key and the control objects are in `leaderboard`. The PR code
runs in `leaderboard-previews`, which has no access to `leaderboard`.

## Security

- The flux-operator applies the generated objects as `preview-applier`. That SA
  can only write `ocirepositories` and `helmreleases` in `leaderboard-previews`.
- helm-controller installs each preview as `preview-installer`. That SA can only
  write what the chart renders, plus Helm release storage.
- The preview pods have no Kubernetes API token.
- The network policy allows ingress from `envoy-external` and kubelet probes
  only. Egress and DNS are allowed only for `login.microsoftonline.com` and
  `graph.microsoft.com`, which the app uses for Azure AD sign-in.
- The previews have the Azure secret. A malicious image can still leak it
  through its own route or through the Microsoft hosts.

## Setup

1. Create a GitHub App with Pull requests read and Metadata read.
   Install it on `faustvii/beth-leaderboard` only. Store these keys in
   Infisical under `/leaderboard`: `GITHUB_APP_ID`,
   `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PRIVATE_KEY`.
2. Store a webhook token at `/leaderboard/GITHUB_WEBHOOK_TOKEN`:

   ```shell
   openssl rand -hex 32
   ```

3. Create the label:

   ```shell
   gh label create preview --repo faustvii/beth-leaderboard
   ```

4. After Flux applies this change, get the webhook path:

   ```shell
   kubectl -n leaderboard get receiver leaderboard-previews -o jsonpath='{.status.webhookPath}'
   ```

   Add a repository webhook on `faustvii/beth-leaderboard`. Use the Flux
   webhook host followed by that path, content type `application/json`, the
   token from step 2, and the `pull_request` event.

5. Add the CI steps in `app-repo-workflows.md` to the app repo.

## Notes

- `${SECRET_DOMAIN}` and `${TZ}` in the template come from Flux postBuild
  substitution.
- Each preview starts with empty data (`emptyDir`, local database).
- The provider exports 10 previews at most.
- Keep the hostname one level below the domain. The gateway certificate is a
  one-level wildcard.
