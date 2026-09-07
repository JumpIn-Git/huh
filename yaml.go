package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadConfig(filePath string) (*yaml.Node, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid YAML format: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("config file is empty")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid config format in %q: expected top-level map, got %v", filePath, root.Kind)
	}
	return root, nil
}

func EnsureKey(mapping *yaml.Node, key string, expectedKind yaml.Kind, expectedTag string) (*yaml.Node, error) {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			valNode := mapping.Content[i+1]
			if valNode.Kind != expectedKind {
				return nil, fmt.Errorf("key %q exists but is of type %v (expected %v)", key, valNode.Kind, expectedKind)
			} else if valNode.Tag != expectedTag {
				return nil, fmt.Errorf("key %q exists but tag is %q, (expected %q)", key, valNode.Tag, expectedTag)
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

// AppendToSequence appends an item to either a standard sequence (!!seq) or an ordered map (!!omap).
// - For !!seq: pass the item in keyOrItem (valNode is ignored/nil).
// - For !!omap: pass the key in keyOrItem and the value in valNode.
// Panics if seqNode is not a SequenceNode or keyOrItem is nil, since misuse indicates a bug.
func AppendToSequence(seqNode *yaml.Node, keyOrItem *yaml.Node, valNode *yaml.Node) {
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		panic("AppendToSequence: target node must be a SequenceNode")
	}
	if keyOrItem == nil {
		panic("AppendToSequence: key/item node cannot be nil")
	}

	switch seqNode.Tag {
	case "!!omap":
		if valNode == nil {
			panic("AppendToSequence: value node is required when appending to an !!omap")
		}
		entryMap := &yaml.Node{
			Kind:    yaml.MappingNode,
			Tag:     "!!map",
			Content: []*yaml.Node{keyOrItem, valNode},
		}
		seqNode.Content = append(seqNode.Content, entryMap)

	default:
		seqNode.Content = append(seqNode.Content, keyOrItem)
	}
}

// SeqContainsInt reports whether a !!seq node contains the given integer value.
func SeqContainsInt(seqNode *yaml.Node, value int) bool {
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		return false
	}
	want := fmt.Sprintf("%d", value)
	for _, item := range seqNode.Content {
		if item.Value == want {
			return true
		}
	}
	return false
}

// AppendIntToSeq appends an integer scalar to a !!seq node, unless it's already present.
// Returns true if the value was newly added, false if it was already present.
// Panics if seqNode is not a SequenceNode.
func AppendIntToSeq(seqNode *yaml.Node, value int) bool {
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		panic("AppendIntToSeq: target node must be a SequenceNode")
	}
	if SeqContainsInt(seqNode, value) {
		return false
	}
	AppendToSequence(seqNode, &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!int",
		Value: fmt.Sprintf("%d", value),
	}, nil)
	return true
}

// SetMapKey sets an integer key on a !!map node to the given string value.
// If the key already exists, its value is replaced.
// Panics if mapNode is not a MappingNode.
func SetMapKey(mapNode *yaml.Node, key int, value string) {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		panic("SetMapKey: target node must be a MappingNode")
	}
	wantKey := fmt.Sprintf("%d", key)
	for i := 0; i < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == wantKey {
			mapNode.Content[i+1] = &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: value,
			}
			return
		}
	}
	mapNode.Content = append(mapNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: wantKey},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
