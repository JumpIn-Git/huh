package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func (a *App) SaveConfig() error {
	b, err := yaml.Marshal(a.Config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.SLSconfigPath, b, 0666); err != nil {
		return err
	}
	return nil
}

func LoadConfig(filePath string) (*yaml.Node, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read config file: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("Invalid YAML format: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("Config file is empty")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("Invalid config format in %q: expected top-level map, got %v", filePath, root.Kind)
	}
	return root, nil
}

func EnsureKey(mapping *yaml.Node, key string, expectedKind yaml.Kind, expectedTag string) (*yaml.Node, error) {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			valNode := mapping.Content[i+1]
			if valNode.Kind != expectedKind {
				if isEmptyScalar(valNode) {
					newVal := &yaml.Node{Kind: expectedKind, Tag: expectedTag}
					mapping.Content[i+1] = newVal
					return newVal, nil
				}
				return nil, fmt.Errorf("Key %q exists but is of type %v (expected %v)", key, valNode.Kind, expectedKind)
			} else if valNode.Tag != expectedTag {
				return nil, fmt.Errorf("Key %q exists but tag is %q, (expected %q)", key, valNode.Tag, expectedTag)
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

// isEmptyScalar reports whether a scalar node represents an empty/null value
// (e.g. a YAML key with no contents, which SLSsteam emits as `Key:`).
func isEmptyScalar(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && (n.Tag == "!!null" || n.Value == "" || n.Value == "null" || n.Value == "~")
}

// SeqContainsInt reports whether a !!seq node contains the given integer value.
// If the value is found, the entry's line comment is set to comment (pass "" to leave unchanged).
func SeqContainsInt(seqNode *yaml.Node, value int, comment string) bool {
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		return false
	}
	want := fmt.Sprintf("%d", value)
	for _, item := range seqNode.Content {
		if item.Value == want {
			item.LineComment = comment
			return true
		}
	}
	return false
}

// AppendIntToSeq appends an integer scalar to a !!seq node, unless it's already present.
// If the value is found, its line comment is set to comment. Otherwise the new
// entry is appended with that line comment.
// Returns true if the value was newly added, false if it was already present.
// Panics if seqNode is not a SequenceNode.
func AppendIntToSeq(seqNode *yaml.Node, value int, comment string) bool {
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		panic("AppendIntToSeq: target node must be a SequenceNode")
	}
	if SeqContainsInt(seqNode, value, comment) {
		return false
	}
	seqNode.Content = append(seqNode.Content, &yaml.Node{
		Kind:        yaml.ScalarNode,
		Tag:         "!!int",
		Value:       fmt.Sprintf("%d", value),
		LineComment: comment,
	})
	return true
}

// SetMapKey sets an integer key on a !!map node to the given string value.
// If the key already exists, its value (and any line comment) is replaced.
// comment is set as a line comment on the value (rendered inline after it);
// pass "" to leave unset.
// Panics if mapNode is not a MappingNode.
func SetMapKey(mapNode *yaml.Node, key int, value, comment string) {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		panic("SetMapKey: target node must be a MappingNode")
	}
	wantKey := fmt.Sprintf("%d", key)
	for i := 0; i < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == wantKey {
			mapNode.Content[i+1] = &yaml.Node{
				Kind:        yaml.ScalarNode,
				Tag:         "!!str",
				Value:       value,
				LineComment: comment,
			}
			return
		}
	}
	mapNode.Content = append(mapNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: wantKey},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, LineComment: comment},
	)
}
