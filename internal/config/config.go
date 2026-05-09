package config

import (
	"fmt"

	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"node_messager/pkg/node"
)

type Config struct {
	Nodes    []node.Node // all sucursales
	HostNode *node.Node  // which node runs locally (nil = dev mode, all nodes local)
	MasterID int         // ID of the sucursal that starts as master
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

	// master_id: which sucursal is initial master
	cfg.MasterID = k.Int("master_id")
	if cfg.MasterID == 0 && len(cfg.Nodes) > 0 {
		cfg.MasterID = cfg.Nodes[0].ID
	}

	// host_id: which sucursal runs locally (VM mode).
	// Looked up from the nodes array — eliminates duplication bugs.
	if k.Exists("host_id") {
		hostID := k.Int("host_id")
		n := nodeByID(cfg.Nodes, hostID)
		if n == nil {
			return Config{}, fmt.Errorf("host_id %d not found in nodes list", hostID)
		}
		cfg.HostNode = n
	} else if k.Exists("host") {
		// legacy fallback: full host object still supported
		var h node.Node
		if err := k.Unmarshal("host", &h); err != nil {
			return Config{}, err
		}
		// prefer matching node from nodes array to avoid data mismatch
		if n := nodeByID(cfg.Nodes, h.ID); n != nil {
			cfg.HostNode = n
		} else {
			cfg.HostNode = &h
		}
	}

	return cfg, nil
}

func nodeByID(nodes []node.Node, id int) *node.Node {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}
