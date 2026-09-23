package cmd

import dsc "github.com/threehook/eamerald/mrld/clients/directory"

type CLI struct {
	Init              InitCmd              `cmd:"" help:"create new database file"`
	Set               SetCmd               `cmd:"" help:"set manifest"`
	Load              LoadCmd              `cmd:"" help:"load data"`
	Sync              SyncCmd              `cmd:"" help:"sync data"`
	MigrateToPostgres MigrateToPostgresCmd `cmd:"" name:"migrate-to-postgres" help:"one-time migration of a boltdb snapshot to a postgres database"`
}

type InitCmd struct {
	Target string `arg:"" help:"db file path, or a postgres:// / postgresql:// connection string"`
}

type SetCmd struct {
	Target   string `arg:"" help:"db file path, or a postgres:// / postgresql:// connection string"`
	Manifest string `arg:"" help:"manifest file path" type:"existingfile"`
}

type LoadCmd struct {
	Target  string `arg:"" help:"db file path, or a postgres:// / postgresql:// connection string"`
	DataDir string `arg:"" help:"data file directory" type:"existingdir"`
}

type SyncCmd struct {
	dsc.Config

	Target string   `arg:"" help:"db file path, or a postgres:// / postgresql:// connection string (must be boltdb-backed; sync targets an edge)"`
	Mode   []string `flag:"" short:"m" enum:"manifest,full,diff,watermark" required:"" help:"sync mode"`
}
