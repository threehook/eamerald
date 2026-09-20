package configure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/threehook/eamerald/internal/xdg"
	"github.com/threehook/eamerald/mrld/cc"
	"github.com/threehook/eamerald/mrld/cmd/common"

	"github.com/itchyny/gojq"
)

type InfoConfigCmd struct {
	Var string `arg:"" optional:"" help:"configuration variable"`
	Raw bool   `flag:"" short:"r" help:"output raw strings"`
}

func (cmd InfoConfigCmd) Run(ctx context.Context) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	// use Info struct when output all, to preserve ordering of root objects.
	if cmd.Var == "" {
		return enc.Encode(cmd.info())
	}

	query, err := gojq.Parse("." + cmd.Var)
	if err != nil {
		return err
	}

	iter := query.Run(cmd.json())

	for {
		v, ok := iter.Next()
		if !ok {
			break
		}

		if err, ok := v.(error); ok {
			if err, ok := err.(*gojq.HaltError); ok && err.Value() == nil { //nolint:errorlint
				break
			}

			return err
		}

		if s, ok := v.(string); ok && cmd.Raw {
			fmt.Fprintln(os.Stdout, s)
		} else {
			_ = enc.Encode(v) //nolint:errchkjson // ok
		}
	}

	return nil
}

type Info struct {
	Environment struct {
		Home          string `json:"home"`
		XdgConfigHome string `json:"xdg_config_home"`
		XdgDataHome   string `json:"xdg_data_home"`
	} `json:"environment"`
	Config struct {
		EameraldCfgDir       string `json:"eamerald_cfg_dir"`
		EameraldCertsDir     string `json:"eamerald_certs_dir"`
		EameraldDataDir      string `json:"eamerald_db_dir"`
		EameraldDecisionsDir string `json:"eamerald_decisions_dir"`
		EameraldTemplateDir  string `json:"eamerald_tmpl_dir"`
		EameraldDir          string `json:"eamerald_dir"`
	} `json:"config"`
	Runtime struct {
		ActiveConfigurationName  string `json:"active_configuration_name"`
		ActiveConfigurationFile  string `json:"active_configuration_file"`
		RunningConfigurationName string `json:"running_configuration_name"`
		RunningConfigurationFile string `json:"running_configuration_file"`
		RunningContainerName     string `json:"running_container_name"`
		EameraldConfigFile       string `json:"eamerald_json"`
	} `json:"runtime"`
	Default struct {
		ContainerRegistry string `json:"container_registry"`
		ContainerImage    string `json:"container_image"`
		ContainerTag      string `json:"container_tag"`
		ContainerPlatform string `json:"container_platform"`
		NoCheck           bool   `json:"eamerald_no_check"`
		NoColor           bool   `json:"eamerald_no_color"`
	} `json:"default"`
	Directory struct {
		DirectorySvc   string `json:"eamerald_directory_svc"`
		DirectoryKey   string `json:"eamerald_directory_key"`
		DirectoryToken string `json:"eamerald_directory_token"`
		Insecure       bool   `json:"eamerald_insecure"`
	} `json:"directory"`
	Authorizer struct {
		AuthorizerSvc   string `json:"eamerald_authorizer_svc"`
		AuthorizerKey   string `json:"eamerald_authorizer_key"`
		AuthorizerToken string `json:"eamerald_authorizer_token"`
		Insecure        bool   `json:"eamerald_insecure"`
	} `json:"authorizer"`
}

func (cmd InfoConfigCmd) info() *Info {
	info := Info{}

	info.Environment.Home = xdg.Home
	info.Environment.XdgConfigHome = xdg.ConfigHome
	info.Environment.XdgDataHome = xdg.DataHome

	info.Config.EameraldCfgDir = cc.GetEameraldCfgDir()
	info.Config.EameraldCertsDir = cc.GetEameraldCertsDir()
	info.Config.EameraldDataDir = cc.GetEameraldDataDir()
	info.Config.EameraldDecisionsDir = cc.GetEameraldDecisionsDir()
	info.Config.EameraldTemplateDir = cc.GetEameraldTemplateDir()
	info.Config.EameraldDir = cc.GetEameraldDir()

	cfg := cc.GetConfig()
	info.Runtime.ActiveConfigurationName = cfg.Active.Config
	info.Runtime.ActiveConfigurationFile = cfg.Active.ConfigFile
	info.Runtime.RunningConfigurationName = cfg.Running.Config
	info.Runtime.RunningConfigurationFile = cfg.Running.ConfigFile
	info.Runtime.RunningContainerName = cfg.Running.ContainerName
	info.Runtime.EameraldConfigFile = filepath.Join(cc.GetEameraldDir(), common.CLIConfigurationFile)

	info.Default.ContainerRegistry = cc.ContainerRegistry()
	info.Default.ContainerImage = cc.ContainerImage()
	info.Default.ContainerTag = cc.ContainerTag()
	info.Default.ContainerPlatform = cc.ContainerPlatform()
	info.Default.NoCheck = cc.NoCheck()
	info.Default.NoColor = cc.NoColor()

	info.Directory.DirectorySvc = cc.DirectorySvc()
	info.Directory.DirectoryKey = cc.DirectoryKey()
	info.Directory.DirectoryToken = cc.DirectoryToken()
	info.Directory.Insecure = cc.Insecure()

	info.Authorizer.AuthorizerSvc = cc.AuthorizerSvc()
	info.Authorizer.AuthorizerKey = cc.AuthorizerKey()
	info.Authorizer.AuthorizerToken = cc.AuthorizerToken()
	info.Authorizer.Insecure = cc.Insecure()

	return &info
}

func (cmd InfoConfigCmd) json() map[string]any {
	var j map[string]any

	buf, err := json.Marshal(cmd.info())
	if err != nil {
		return map[string]any{}
	}

	if err := json.Unmarshal(buf, &j); err != nil {
		return map[string]any{}
	}

	return j
}
