package config

type templateParams struct {
	Version           int    // must be 2
	ConfigName        string //
	PolicyRegistry    string //
	PolicyName        string //
	Resource          string //
	Authorization     string //
	LocalPolicy       bool   //
	EdgeDirectory     bool   // OBSOLETE
	SeedMetadata      bool   // OBSOLETE
	EnableDirectoryV2 bool   // OBSOLETE
	RegistryService   string //
	RegistryImage     string //
	RegistryTag       string //
}

const LocalImageTemplate string = templatePreamble + opaLocalPolicyImage +
	adlDecisionLoggerPlugin + asertoEdgePlugin + gitPolicySourcePlugin + entraDirectorySyncPlugin

const RemoteImageTemplate string = templatePreamble + opaRemotePolicyImage +
	adlDecisionLoggerPlugin + asertoEdgePlugin + gitPolicySourcePlugin + entraDirectorySyncPlugin

const templatePreamble string = `# yaml-language-server: $schema=https://topaz.sh/schema/config.json
---
# config schema version
version: {{ .Version }}

# logger settings.
logging:
  prod: true
  log_level: info
  grpc_log_level: info

# edge directory configuration.
directory:
  db_path: '${EAMERALD_DB_DIR}/{{ .ConfigName }}.db'
  request_timeout: 5s # set as default, 5 secs.

# remote directory is used to resolve the identity for the authorizer.
remote_directory:
  address: "0.0.0.0:9292" # set as default, it should be the same as the reader as we resolve the identity from the local directory service.
  insecure: true
  no_tls: false
  no_proxy: false
  api_key: ""
  token: ""
  client_cert_path: ""
  client_key_path: ""
  ca_cert_path: ""
  headers:

# default jwt validation configuration
jwt:
  acceptable_time_skew_seconds: 5 # set as default, 5 secs
  allowed_issuers: # NOTE: if empty, any issuer is accepted !!!
  #  - "https://issuer.example.com/"
  cache_refresh_min_interval: 5m  # set as default, 5 minutes
  cache_refresh_max_interval: 15m # set as default, 15 minutes
  expected_audience: "" # set as default, empty

# authentication configuration
auth:
  keys:
    # - "<API key>"
    # - "<Password>"
  options:
    default:
      enable_api_key: false
      enable_anonymous: true
    overrides:
      paths:
        - /aserto.authorizer.v2.Authorizer/Info
        - /grpc.reflection.v1.ServerReflection/ServerReflectionInfo
        - /grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo
      override:
        enable_api_key: false
        enable_anonymous: true

api:
  health:
    listen_address: "0.0.0.0:9494"

  metrics:
    listen_address: "0.0.0.0:9696"
    zpages: true

  services:
    console:
      grpc:
        listen_address: "0.0.0.0:8081"
        fqdn: ""
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/grpc.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/grpc.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/grpc-ca.crt'
      gateway:
        listen_address: "0.0.0.0:8080"
        fqdn: ""
        allowed_headers:
        - "Authorization"
        - "Content-Type"
        - "If-Match"
        - "If-None-Match"
        - "Depth"
        allowed_methods:
        - "GET"
        - "POST"
        - "HEAD"
        - "DELETE"
        - "PUT"
        - "PATCH"
        - "PROFIND"
        - "MKCOL"
        - "COPY"
        - "MOVE"
        allowed_origins:
        - http://localhost
        - http://localhost:*
        - https://localhost
        - https://localhost:*
        - https://0.0.0.0:*
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/gateway.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/gateway.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/gateway-ca.crt'
        http: false
        read_timeout: 2s
        read_header_timeout: 2s
        write_timeout: 2s
        idle_timeout: 30s

    model:
      grpc:
        listen_address: "0.0.0.0:9292"
        fqdn: ""
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/grpc.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/grpc.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/grpc-ca.crt'
      gateway:
        listen_address: "0.0.0.0:9393"
        fqdn: ""
        allowed_headers:
        - "Authorization"
        - "Content-Type"
        - "If-Match"
        - "If-None-Match"
        - "Depth"
        allowed_methods:
        - "GET"
        - "POST"
        - "HEAD"
        - "DELETE"
        - "PUT"
        - "PATCH"
        - "PROFIND"
        - "MKCOL"
        - "COPY"
        - "MOVE"
        allowed_origins:
        - http://localhost
        - http://localhost:*
        - https://localhost
        - https://localhost:*
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/gateway.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/gateway.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/gateway-ca.crt'
        http: false
        read_timeout: 2s
        read_header_timeout: 2s
        write_timeout: 2s
        idle_timeout: 30s

    reader:
      needs:
        - model
      grpc:
        listen_address: "0.0.0.0:9292"
        fqdn: ""
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/grpc.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/grpc.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/grpc-ca.crt'
      gateway:
        listen_address: "0.0.0.0:9393"
        fqdn: ""
        allowed_headers:
        - "Authorization"
        - "Content-Type"
        - "If-Match"
        - "If-None-Match"
        - "Depth"
        allowed_methods:
        - "GET"
        - "POST"
        - "HEAD"
        - "DELETE"
        - "PUT"
        - "PATCH"
        - "PROFIND"
        - "MKCOL"
        - "COPY"
        - "MOVE"
        allowed_origins:
        - http://localhost
        - http://localhost:*
        - https://localhost
        - https://localhost:*
        - https://0.0.0.0:*
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/gateway.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/gateway.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/gateway-ca.crt'
        http: false
        read_timeout: 2s # default 2 seconds
        read_header_timeout: 2s
        write_timeout: 2s
        idle_timeout: 30s # default 30 seconds

    writer:
      needs:
        - model
      grpc:
        listen_address: "0.0.0.0:9292"
        fqdn: ""
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/grpc.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/grpc.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/grpc-ca.crt'
      gateway:
        listen_address: "0.0.0.0:9393"
        fqdn: ""
        allowed_headers:
        - "Authorization"
        - "Content-Type"
        - "If-Match"
        - "If-None-Match"
        - "Depth"
        allowed_methods:
        - "GET"
        - "POST"
        - "HEAD"
        - "DELETE"
        - "PUT"
        - "PATCH"
        - "PROFIND"
        - "MKCOL"
        - "COPY"
        - "MOVE"
        allowed_origins:
        - http://localhost
        - http://localhost:*
        - https://localhost
        - https://localhost:*
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/gateway.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/gateway.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/gateway-ca.crt'
        http: false
        read_timeout: 2s
        read_header_timeout: 2s
        write_timeout: 2s
        idle_timeout: 30s

    exporter:
      grpc:
        listen_address: "0.0.0.0:9292"
        fqdn: ""
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/grpc.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/grpc.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/grpc-ca.crt'

    importer:
      needs:
        - model
      grpc:
        listen_address: "0.0.0.0:9292"
        fqdn: ""
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/grpc.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/grpc.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/grpc-ca.crt'

    authorizer:
      needs:
        - reader
      grpc:
        connection_timeout_seconds: 2
        listen_address: "0.0.0.0:8282"
        fqdn: ""
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/grpc.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/grpc.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/grpc-ca.crt'
      gateway:
        listen_address: "0.0.0.0:8383"
        fqdn: ""
        allowed_headers:
        - "Authorization"
        - "Content-Type"
        - "If-Match"
        - "If-None-Match"
        - "Depth"
        allowed_methods:
        - "GET"
        - "POST"
        - "HEAD"
        - "DELETE"
        - "PUT"
        - "PATCH"
        - "PROFIND"
        - "MKCOL"
        - "COPY"
        - "MOVE"
        allowed_origins:
        - http://localhost
        - http://localhost:*
        - https://localhost
        - https://localhost:*
        - https://0.0.0.0:*
        certs:
          tls_key_path: '${EAMERALD_CERTS_DIR}/gateway.key'
          tls_cert_path: '${EAMERALD_CERTS_DIR}/gateway.crt'
          tls_ca_cert_path: '${EAMERALD_CERTS_DIR}/gateway-ca.crt'
        http: false
        read_timeout: 2s
        read_header_timeout: 2s
        write_timeout: 2s
        idle_timeout: 30s
`

const opaLocalPolicyImage string = `
opa:
  instance_id: "-"
  policy_root: ""                 # package root the AuthZEN Access API falls back to; only needed when the bundle has several.
  graceful_shutdown_period_seconds: 2
  # max_plugin_wait_time_seconds: 30 set as default
  local_bundles:
    local_policy_image: {{ .Resource }}
    watch: true
    skip_verification: true
  config:
    decision_logs:
      console: false
    plugins:
`

const opaRemotePolicyImage string = `
opa:
  instance_id: "-"
  policy_root: ""                 # package root the AuthZEN Access API falls back to; only needed when the bundle has several.
  graceful_shutdown_period_seconds: 2
  # max_plugin_wait_time_seconds: 30 set as default
  local_bundles:
    paths: []
    skip_verification: true
  config:
    services:
      policy-registry:
        url: "{{ .PolicyRegistry }}"
        type: "oci"
        response_header_timeout_seconds: 15
    bundles:
      {{ .PolicyName }}:
        service: policy-registry
        resource: "{{ .Resource }}"
        persist: false
        config:
          polling:
            min_delay_seconds: 60
            max_delay_seconds: 120
    decision_logs:
      console: false
    plugins:
`

const adlDecisionLoggerPlugin string = `
      # logius adl level 1 decision logger plugin configuration
      adl_decision_logger:
        enabled: false
        output: 'stdout,otlp'        # comma-separated: stdout, otlp, or both. Defaults to both when unset. Settable via ${LOG_OUTPUT}.
        otlp:
          endpoint: ''               # otlp/gRPC collector address, e.g. a Grafana Alloy receiver: localhost:4317
          insecure: true             # disable TLS - typical for a same-cluster/sidecar collector
        resource: {}                 # producer identity, e.g. {service.name: eamerald}. REQUIRED when records are aggregated across organisations.
        resource_context:            # keys read from an authorizer resource context to fill the AuthZEN resource
          type_key: 'object_type'
          id_key: 'object_id'
`

const asertoEdgePlugin string = `
      # aserto edge directory sync plugin configuration
      aserto_edge:
        enabled: false 
        addr: ""                    # gRPC directory service address.
        apikey: ""                  # directory API key.
        timeout: 5                  # gRPC connection timeout in seconds.
        sync_interval: 1            # sync run interval in minutes.
        insecure: true              # when using TLS connections, skip verification of the server certificate. 
        page_size: 0                # deprecated: no longer used.
        client_cert_path: ""        # when using mTLS connections, ClientCertPath is the path of the client's certificate file.
        client_key_path: ""         # when using mTLS connections, ClientKeyPath is the path of the client's private key file.
        no_tls: false               # disable TLS and use a plaintext connection.
        no_proxy: false             # bypasses any configured HTTP proxy.
        headers:                    # additional headers to include in requests to the service.
`

const gitPolicySourcePlugin string = `
      # git policy source plugin configuration
      git:
        enabled: false
        repo: ""                        # git remote URL, e.g. "https://github.com/org/repo.git" or "git@github.com:org/repo.git".
        ref: "refs/heads/main"          # git reference to track: branch, tag, or full ref name.
        path: ""                        # subdirectory within the repo containing the policy bundle; empty means repo root.
        cache_dir: ""                   # local directory used to clone/checkout the repo; auto-derived under ~/.policy/git when empty.
        poll_interval_seconds: 60       # how often to fetch and check for updates.
        insecure_skip_tls_verify: false # skip TLS certificate verification for HTTPS remotes.
        auth:
          username: ""                  # basic-auth / PAT username for HTTPS remotes.
          token: ""                     # PAT / password for HTTPS remotes.
          ssh_key_path: ""              # path to a private key file, for SSH remotes.
          ssh_key_passphrase: ""        # passphrase for the private key, if any.
          known_hosts_path: ""          # optional known_hosts file used to verify the SSH host key.
        skip_verification: true         # skip bundle signature verification.
        verification_config:            # bundle signature verification config; see OPA docs for bundle signing.
`

const entraDirectorySyncPlugin string = `
      # microsoft entra id (azure ad) directory sync plugin configuration
      entra:
        enabled: false
        tenant_id: ""                   # entra id tenant ID.
        client_id: ""                   # app registration (client) ID; needs admin-consented User.Read.All / Group.Read.All perms.
        client_secret: ""               # app registration client secret.
        poll_interval_seconds: 300      # how often to sync users and groups.
        user_object_type: "user"        # directory object type synced users are written as.
        group_object_type: "group"      # directory object type synced groups are written as.
        member_relation: "member"       # directory relation name used for group membership.
`
