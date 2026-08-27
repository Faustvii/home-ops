# nut-client upsmon config. Templated (.tpl) because the three credentials are
# interpolated into a single line. topf resolves .Data.* from topf.yaml, where the
# values are ref+sops:// pointers into the encrypted data.sops.yaml (old talenv.sops.yaml).
# NOTE: talhelper used ${upsmonHost} envsubst; topf uses Go templates -> {{ .Data.x }}.
apiVersion: v1alpha1
kind: ExtensionServiceConfig
name: nut-client
configFiles:
  - content: |-
      MONITOR ups@{{ .Data.upsmonHost }} 1 {{ .Data.upsmonUser }} {{ .Data.upsmonPass }} secondary
      SHUTDOWNCMD "/sbin/poweroff"
    mountPath: /usr/local/etc/nut/upsmon.conf
