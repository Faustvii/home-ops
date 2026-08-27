# Typed HostnameConfig, templated per node (topf sets .Node.Host from topf.yaml).
apiVersion: v1alpha1
kind: HostnameConfig
hostname: {{ .Node.Host }}
