package main

import (
	"fmt"
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

// encodeYAMLWithMultiline encodes a value to YAML with multiline strings properly formatted
func encodeYAMLWithMultiline(buf *strings.Builder, value interface{}) error {
	// First, marshal to a YAML node
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return fmt.Errorf("failed to encode to node: %w", err)
	}

	// Process the node to set multiline style
	forceMultilineInYAML(&node)

	// Now encode the processed node
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
