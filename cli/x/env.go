package x

// Environment variable name constants

const (
	EnvEameraldDir                string = "EAMERALD_DIR"
	EnvEameraldCfgDir             string = "EAMERALD_CFG_DIR"
	EnvEameraldCertsDir           string = "EAMERALD_CERTS_DIR"
	EnvEameraldDBDir              string = "EAMERALD_DB_DIR"
	EnvEameraldTmplDir            string = "EAMERALD_TMPL_DIR"
	EnvEameraldTmplURL            string = "EAMERALD_TMPL_URL"
	EnvEameraldDecisionsDir       string = "EAMERALD_DECISIONS_DIR"
	EnvEameraldAuthorizerSvc      string = "EAMERALD_AUTHORIZER_SVC"
	EnvEameraldAuthorizerKey      string = "EAMERALD_AUTHORIZER_KEY"
	EnvEameraldAuthorizerToken    string = "EAMERALD_AUTHORIZER_TOKEN" //nolint:gosec // G101: this is an env var name, not a credential
	EnvEameraldDirectorySvc       string = "EAMERALD_DIRECTORY_SVC"
	EnvEameraldDirectoryKey       string = "EAMERALD_DIRECTORY_KEY"
	EnvEameraldDirectoryToken     string = "EAMERALD_DIRECTORY_TOKEN" //nolint:gosec // G101: this is an env var name, not a credential
	EnvEameraldInsecure           string = "EAMERALD_INSECURE"
	EnvEameraldPlaintext          string = "EAMERALD_PLAINTEXT"
	EnvEameraldTimeout            string = "EAMERALD_TIMEOUT"
	EnvEameraldNoCheck            string = "EAMERALD_NO_CHECK"
	EnvEameraldNoColor            string = "EAMERALD_NO_COLOR"
	EnvEameraldFeatureFlag        string = "EAMERALD_FFLAG"
	EnvAsertoHostName             string = "ASERTO_HOSTNAME"
	EnvHostName                   string = "HOSTNAME"
	EnvContainer                  string = "CONTAINER"
	EnvContainerRegistry          string = "CONTAINER_REGISTRY"
	EnvContainerImage             string = "CONTAINER_IMAGE"
	EnvContainerTag               string = "CONTAINER_TAG"
	EnvContainerPlatform          string = "CONTAINER_PLATFORM"
	EnvContainerName              string = "CONTAINER_NAME"
	EnvPolicyFileStoreRoot        string = "POLICY_FILE_STORE_ROOT"
	EnvEameraldRunningInContainer string = "EAMERALD_RUNNING_IN_CONTAINER"
)
