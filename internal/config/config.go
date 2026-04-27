package config

import (
	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"node_messager/pkg/node"
)

func LoadNodes(path string) ([]node.Node, error) {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), kjson.Parser()); err != nil {
		return nil, err
	}
	var nodes []node.Node
	if err := k.Unmarshal("nodes", &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}
