# App repo CI (faustvii/beth-leaderboard)

The preview CI lives in the app repo. Flux does not apply this file.

CI does three things for each PR:

1. It pushes the image with the PR head SHA as the tag.
2. It adds the `preview` label, which starts the deploy.
3. It records a GitHub Deployment with the preview URL.

Set the repository variable `PREVIEW_DOMAIN` to the cluster domain.

## Image tag

In `.github/workflows/build.yml`, add this tag to the image build step:

```yaml
tags: |
  ghcr.io/faustvii/beth-leaderboard:${{ github.event.pull_request.head.sha }}
```

## Preview job

Add this job to `build.yml`, after the image build job:

```yaml
  preview:
    needs: build-and-publish-docker
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      deployments: write
      pull-requests: write
    environment:
      name: preview/pr-${{ github.event.pull_request.number }}
      url: https://pr-${{ github.event.pull_request.number }}-leaderboard.${{ vars.PREVIEW_DOMAIN }}
    steps:
      - name: Create deployment
        id: deploy
        uses: chrnorm/deployment-action@v2
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          environment: preview/pr-${{ github.event.pull_request.number }}
          sha: ${{ github.event.pull_request.head.sha }}
          ref: ${{ github.head_ref }}
          auto-merge: false
          transient_environment: true

      # The image exists now, so the label is safe to add.
      - name: Trigger Flux preview
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
        run: gh pr edit "$PR_NUMBER" --repo "${{ github.repository }}" --add-label preview

      - name: Wait for the preview to answer
        run: |
          curl --retry 30 --retry-delay 5 --retry-all-errors -fsS \
            "https://pr-${{ github.event.pull_request.number }}-leaderboard.${{ vars.PREVIEW_DOMAIN }}/static/styles.css" -o /dev/null

      - name: Report deployment status
        if: always() && steps.deploy.outputs.deployment_id
        uses: chrnorm/deployment-status@v2
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          deployment-id: ${{ steps.deploy.outputs.deployment_id }}
          state: ${{ job.status == 'success' && 'success' || 'failure' }}
          environment-url: https://pr-${{ github.event.pull_request.number }}-leaderboard.${{ vars.PREVIEW_DOMAIN }}
          log-url: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
```
