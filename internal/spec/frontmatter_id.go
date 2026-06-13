package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// EnsureCroftyID makes sure the front matter has crofty.id. If absent, newID is
// inserted (preserving the body and existing keys/comments) and the updated
// content is returned with created=true. If already present, the existing id is
// returned and out equals src unchanged.
func EnsureCroftyID(src []byte, newID string) (id string, created bool, out []byte, err error) {
	block, body, err := split(src)
	if err != nil {
		return "", false, nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(block), &doc); err != nil {
		return "", false, nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", false, nil, fmt.Errorf("frontmatter is not a mapping")
	}
	root := doc.Content[0]

	switch crofty := mapValue(root, "crofty"); {
	case crofty == nil:
		root.Content = append(root.Content,
			scalarNode("crofty"),
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map",
				Content: []*yaml.Node{scalarNode("id"), scalarNode(newID)}},
		)
		id, created = newID, true
	case crofty.Kind == yaml.MappingNode:
		if idNode := mapValue(crofty, "id"); idNode != nil {
			return idNode.Value, false, src, nil
		}
		crofty.Content = append(crofty.Content, scalarNode("id"), scalarNode(newID))
		id, created = newID, true
	default:
		return "", false, nil, fmt.Errorf("crofty: in frontmatter is not a mapping")
	}

	newBlock, err := yaml.Marshal(&doc)
	if err != nil {
		return "", false, nil, err
	}
	return id, created, []byte("---\n" + string(newBlock) + "---\n" + body), nil
}

// EnsureCroftyIDFile ensures crofty.id in the file at path, writing it back only
// if a new id was assigned. Returns the id and whether it was newly created.
func EnsureCroftyIDFile(path, newID string) (id string, created bool, err error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	id, created, out, err := EnsureCroftyID(src, newID)
	if err != nil {
		return "", false, err
	}
	if created {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return "", false, err
		}
	}
	return id, created, nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalarNode(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}
