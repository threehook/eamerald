package authorizer

import (
	"context"
	"time"

	client "github.com/aserto-dev/go-aserto"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/threehook/eamerald/cli/clients"
)

type Config struct {
	Host      string            `flag:"host" short:"H" default:"${authorizer_svc}" env:"EAMERALD_AUTHORIZER_SVC" help:"authorizer service address"`
	APIKey    string            `flag:"api-key" short:"k" default:"${authorizer_key}" env:"EAMERALD_AUTHORIZER_KEY" help:"authorizer API key"`
	Token     string            `flag:"token" default:"${authorizer_token}" env:"EAMERALD_AUTHORIZER_TOKEN" help:"authorizer OAuth2.0 token" hidden:""`
	Insecure  bool              `flag:"insecure" short:"i" default:"${insecure}" env:"EAMERALD_INSECURE" help:"skip TLS verification"`
	Plaintext bool              `flag:"plaintext" short:"P" default:"${plaintext}" env:"EAMERALD_PLAINTEXT" help:"use plain-text HTTP/2 (no TLS)"`
	Headers   map[string]string `flag:"headers" env:"EAMERALD_AUTHORIZER_HEADERS" help:"additional headers to send to the authorizer service"`
	Timeout   time.Duration     `flag:"timeout" short:"T" default:"${timeout}" env:"EAMERALD_TIMEOUT" help:"command timeout"`
}

var _ clients.Config = &Config{}

type Client struct {
	conn   *grpc.ClientConn
	Access dsa.AccessClient
}

func New(conn *grpc.ClientConn) *Client {
	return &Client{
		conn:   conn,
		Access: dsa.NewAccessClient(conn),
	}
}

func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	conn, err := cfg.Connect(ctx)
	if err != nil {
		return nil, err
	}

	return New(conn), nil
}

func (cfg *Config) Connect(ctx context.Context) (*grpc.ClientConn, error) {
	if cfg.Host == "" {
		return nil, errors.Errorf("no host specified")
	}

	if ok, err := clients.Validate(ctx, cfg); !ok {
		return nil, err
	}

	return cfg.ClientConfig().Connect()
}

func (cfg *Config) ClientConfig() *client.Config {
	return &client.Config{
		Address:  cfg.Host,
		Insecure: cfg.Insecure,
		NoTLS:    cfg.Plaintext,
		APIKey:   cfg.APIKey,
		Token:    cfg.Token,
		Headers:  cfg.Headers,
	}
}

func (cfg *Config) CommandTimeout() time.Duration {
	return cfg.Timeout
}

func (cfg *Config) Invoke(ctx context.Context, method string, args any, reply any) error {
	con, err := cfg.Connect(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get gRPC client connection")
	}

	if err := con.Invoke(ctx, method, args, reply, []grpc.CallOption{grpc.StaticMethod()}...); err != nil {
		return errors.Wrapf(err, "invoke method %s failed", method)
	}

	return nil
}
