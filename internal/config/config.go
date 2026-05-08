package config

import (
	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"node_messager/pkg/node"
)

type Config struct {
	Nodes      []node.Node // sucursales (branches), no includes master
	MasterNode *node.Node  // sucursal 0 — orchestrator
	HostNode   *node.Node  // which node runs locally (VM mode)
	MasterID   int
}

// AllNodes returns sucursales + master combined.
func (c *Config) AllNodes() []node.Node {
	all := append([]node.Node{}, c.Nodes...)
	if c.MasterNode != nil {
		all = append(all, *c.MasterNode)
	}
	return all
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
	if k.Exists("master") {
		var m node.Node
		if err := k.Unmarshal("master", &m); err != nil {
			return Config{}, err
		}
		cfg.MasterNode = &m
		cfg.MasterID = m.ID
	}
	if k.Exists("host") {
		var h node.Node
		if err := k.Unmarshal("host", &h); err != nil {
			return Config{}, err
		}
		cfg.HostNode = &h
	}
	// fallback: if no master block, use master_id field pointing to a sucursal
	if cfg.MasterNode == nil {
		cfg.MasterID = k.Int("master_id")
		if cfg.MasterID == 0 && len(cfg.Nodes) > 0 {
			cfg.MasterID = cfg.Nodes[0].ID
		}
	}
	return cfg, nil
}
