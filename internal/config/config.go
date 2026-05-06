package config

import (
	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"node_messager/pkg/node"
)

type Config struct {
	Nodes    []node.Node
	HostNode *node.Node
	MasterID int
}

func LoadConfig(path string) (Config, error) {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), kjson.Parser()); err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := k.Unmarshal("nodes", &cfg.Nodes); err != nil {
		return Config{}, err
	}
	if k.Exists("host") {
		var h node.Node
		if err := k.Unmarshal("host", &h); err != nil {
			return Config{}, err
		}
		cfg.HostNode = &h
	}
	cfg.MasterID = k.Int("master_id")
	if cfg.MasterID == 0 && len(cfg.Nodes) > 0 {
		cfg.MasterID = cfg.Nodes[0].ID // default to first node
	}
	return cfg, nil
}
