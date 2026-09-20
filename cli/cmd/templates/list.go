package templates

import (
	"context"
	"os"

	"github.com/threehook/eamerald/cli/constants"
	"github.com/threehook/eamerald/cli/table"
)

type ListTemplatesCmd struct {
	Legacy       bool   `optional:"" default:"false" help:"use legacy templates"`
	TemplatesURL string `optional:"" default:"${eamerald_tmpl_url}" env:"EAMERALD_TMPL_URL" help:"URL of template catalog"`
}

func (cmd *ListTemplatesCmd) Run(ctx context.Context) error {
	if cmd.Legacy {
		cmd.TemplatesURL = constants.TopazTmplV32URL
	}

	ctlg, err := getCatalog(cmd.TemplatesURL)
	if err != nil {
		return err
	}

	data := [][]any{}
	for n, t := range ctlg {
		data = append(data, []any{n, t.ShortDescription, t.DocumentationURL})
	}

	t := table.New(os.Stdout)

	t.Header(colName, colDescription, colDocumentation)
	t.Bulk(data)
	t.Render()

	return nil
}
