package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func EnsureKey(mapping *yaml.Node, key string, expectedKind yaml.Kind, expectedTag string) (*yaml.Node, error) {
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("provided target node is %v, expected MappingNode", mapping.Kind)
	}

	for i := 0; i < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		if keyNode.Value == key {
			valNode := mapping.Content[i+1]
			if valNode.Kind != expectedKind {
				return nil, fmt.Errorf("key %q exists but is of type %v (expected %v)", key, valNode.Kind, expectedKind)
			}
			return valNode, nil
		}
	}

	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: key,
	}
	valNode := &yaml.Node{
		Kind: expectedKind,
		Tag:  expectedTag,
	}

	mapping.Content = append(mapping.Content, keyNode, valNode)
	return valNode, nil
}
