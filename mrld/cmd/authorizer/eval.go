package authorizer

import (
	"context"
	"os"

	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/threehook/eamerald/mrld/clients"
	azc "github.com/threehook/eamerald/mrld/clients/authorizer"
	"github.com/threehook/eamerald/mrld/jsonx"
	"google.golang.org/protobuf/proto"
)

type EvalCmd struct {
	clients.RequestArgs
	azc.Config

	req  dsa.EvaluationRequest
	resp dsa.EvaluationResponse
}

func (cmd *EvalCmd) Run(ctx context.Context) error {
	if cmd.Template {
		return jsonx.OutputJSONPB(os.Stdout, cmd.template())
	}

	if err := cmd.Process(&cmd.req, cmd.template); err != nil {
		return err
	}

	if err := cmd.Invoke(ctx, dsa.Access_Evaluation_FullMethodName, &cmd.req, &cmd.resp); err != nil {
		return err
	}

	return jsonx.OutputJSONPB(os.Stdout, &cmd.resp)
}

func (cmd *EvalCmd) template() proto.Message {
	return &dsa.EvaluationRequest{
		Subject:  &dsa.Subject{Type: "", Id: ""},
		Action:   &dsa.Action{Name: allowed},
		Resource: &dsa.Resource{Type: "", Id: ""},
	}
}
