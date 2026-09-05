apiVersion: v1alpha1
kind: ExtensionServiceConfig
name: nut-client
configFiles:
  - content: |-
      MONITOR ups@{{ .Data.upsmonHost }} 1 {{ .Data.upsmonUser }} {{ .Data.upsmonPass }} secondary
      SHUTDOWNCMD "/sbin/poweroff"
    mountPath: /usr/local/etc/nut/upsmon.conf
