package main

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MultilineString is a custom type that forces multiline YAML formatting for strings with line breaks
type MultilineString string

// MarshalYAML implements yaml.Marshaler to force literal style for multiline strings
func (ms MultilineString) MarshalYAML() (interface{}, error) {
	s := string(ms)
	if strings.Contains(s, "\n") {
		// Create a node with literal style (|) for multiline strings
		node := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Style: yaml.LiteralStyle, // Use | style for multiline
			Value: s,
		}
		return node, nil
	}
	// Return as regular string if no line breaks
	return s, nil
}

// forceMultilineInYAML recursively processes YAML nodes and sets literal style for strings with line breaks
func forceMultilineInYAML(node *yaml.Node) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		// Process document content
		for _, child := range node.Content {
			forceMultilineInYAML(child)
		}
	case yaml.MappingNode:
		// Sort the mapping node to maintain consistent ordering
		sortMappingNode(node)
		// Process all key-value pairs in the mapping
		for _, child := range node.Content {
			forceMultilineInYAML(child)
		}
	case yaml.SequenceNode:
		// Process all items in the sequence
		for _, child := range node.Content {
			forceMultilineInYAML(child)
		}
	case yaml.ScalarNode:
		// If this is a string scalar with line breaks, use literal style
		if node.Tag == "!!str" && strings.Contains(node.Value, "\n") {
			node.Style = yaml.LiteralStyle
		}
	}
}

// sortMappingNode sorts the key-value pairs in a mapping node alphabetically by key
func sortMappingNode(node *yaml.Node) {
	if node.Kind != yaml.MappingNode || len(node.Content) == 0 {
		return
	}

	// Mapping nodes have alternating key-value pairs in Content
	// We need to sort by keys while keeping key-value pairs together
	type keyValuePair struct {
		key   *yaml.Node
		value *yaml.Node
	}

	var pairs []keyValuePair
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			pairs = append(pairs, keyValuePair{
				key:   node.Content[i],
				value: node.Content[i+1],
			})
		}
	}

	// Sort pairs by key value
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].key.Value < pairs[j].key.Value
	})

	// Rebuild the Content slice with sorted pairs
	newContent := make([]*yaml.Node, 0, len(node.Content))
	for _, pair := range pairs {
		newContent = append(newContent, pair.key, pair.value)
	}
	node.Content = newContent
}

// encodeYAMLWithMultiline encodes a value to YAML with multiline strings properly formatted
// and preserves the order of map entries
func encodeYAMLWithMultiline(buf *strings.Builder, value interface{}) error {
	// First, marshal to a YAML node
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return fmt.Errorf("failed to encode to node: %w", err)
	}

	// Process the node to set multiline style
	forceMultilineInYAML(&node)

	// Now encode the processed node with proper indentation
	encoder := yaml.NewEncoder(buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(&node); err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return fmt.Errorf("failed to close encoder: %w", err)
	}

	return nil
}
