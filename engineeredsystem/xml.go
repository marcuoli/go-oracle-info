package engineeredsystem

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type xmlPart struct {
	child *xmlNode
	text  string
}
type xmlNode struct {
	name    xml.Name
	attrs   []xml.Attr
	parts   []xmlPart
	ordinal int
}

func xmlName(name xml.Name) string {
	if name.Space != "" {
		return "{" + name.Space + "}" + name.Local
	}
	return name.Local
}

// DecodeXML returns only a complete document's properties. The caller retains
// Payload independently even on failure and can keep other valid documents.
// encoding/xml's strict decoder does not fetch DTDs or external entities. No
// entity map, lossy CLOB shortening or XML shape allowlist is introduced here.
func DecodeXML(payload Payload) ([]Property, error) {
	property := func(name, value string, null bool) Property {
		return Property{InstanceID: payload.InstanceID, CellHash: payload.CellHash, CellPath: payload.CellPath, CellType: payload.CellType, ComponentType: payload.SourceType, ComponentID: payload.ObjectName, Name: name, Value: value, Source: payload.Source, SourceType: payload.SourceType, Null: null, ObservedAt: payload.ObservedAt}
	}
	if payload.Null {
		if payload.XML != "" {
			return nil, fmt.Errorf("NULL XML payload contains source text")
		}
		p := property("XML", "", true)
		p.Ordinal = 1
		return []Property{p}, nil
	}
	decoder := xml.NewDecoder(strings.NewReader(payload.XML))
	var root *xmlNode
	var stack []*xmlNode
	// Child ordinals preserve every sibling, including repeated names and
	// namespace-qualified names; a map never selects one observation as winner.
	counts := []map[string]int{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid engineered-system XML: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			name := xmlName(token.Name)
			n := &xmlNode{name: token.Name, attrs: append([]xml.Attr(nil), token.Attr...)}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("engineered-system XML has multiple root elements")
				}
				n.ordinal = 1
				root = n
			} else {
				parent := stack[len(stack)-1]
				siblings := counts[len(counts)-1]
				siblings[name]++
				n.ordinal = siblings[name]
				parent.parts = append(parent.parts, xmlPart{child: n})
			}
			stack = append(stack, n)
			counts = append(counts, map[string]int{})
		case xml.EndElement:
			stack = stack[:len(stack)-1]
			counts = counts[:len(counts)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(token)) != "" {
					return nil, fmt.Errorf("engineered-system XML contains text outside its root")
				}
				continue
			}
			n := stack[len(stack)-1]
			if len(n.parts) > 0 && n.parts[len(n.parts)-1].child == nil {
				n.parts[len(n.parts)-1].text += string(token)
			} else {
				n.parts = append(n.parts, xmlPart{text: string(token)})
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, fmt.Errorf("engineered-system XML has no complete root")
	}
	type frame struct {
		node                                     *xmlNode
		componentType, componentID, status, unit string
		initialized                              bool
		partIndex, textOrdinal, children         int
	}
	frames := []frame{{node: root, componentType: payload.SourceType, componentID: payload.ObjectName}}
	var properties []Property
	for len(frames) > 0 {
		current := &frames[len(frames)-1]
		node := current.node
		// Build a path only for an emitted property or component identifier.
		// Retaining every ancestor path would use quadratic space for deep XML.
		nodePath := func() string {
			var path strings.Builder
			for _, ancestor := range frames {
				path.WriteByte('/')
				path.WriteString(xmlName(ancestor.node.name))
				path.WriteByte('[')
				path.WriteString(strconv.Itoa(ancestor.node.ordinal))
				path.WriteByte(']')
			}
			return path.String()
		}
		add := func(name, value string) {
			p := property(name, value, false)
			p.ComponentType, p.ComponentID, p.ReportedStatus, p.Unit = current.componentType, current.componentID, current.status, current.unit
			p.Ordinal = len(properties) + 1
			properties = append(properties, p)
		}
		if !current.initialized {
			current.initialized = true
			if current.componentType == "" {
				current.componentType = node.name.Local
			}
			if current.componentID == "" {
				current.componentID = nodePath()
			}
			if isComponent(node.name.Local) {
				current.componentType = node.name.Local
				current.componentID = componentName(node)
				if current.componentID == "" {
					current.componentID = nodePath()
				}
				current.status = ""
			}
			// Only an explicit unit on this element labels its property value;
			// applying a parent unit to arbitrary child fields would invent units.
			current.unit = ""
			for _, attr := range node.attrs {
				if attr.Name.Space == "" && (attr.Name.Local == "unit" || attr.Name.Local == "units") {
					current.unit = attr.Value
				}
			}
			// Preserve reported status literally. Conflicting status observations
			// remain separate properties; none is selected as a component winner.
			statuses := []string{}
			for _, part := range node.parts {
				if part.child != nil {
					current.children++
					if part.child.name.Space == node.name.Space && strings.EqualFold(part.child.name.Local, "status") {
						statuses = append(statuses, nodeText(part.child))
					}
				}
			}
			if len(statuses) > 0 {
				current.status = statuses[0]
				for _, status := range statuses[1:] {
					if status != current.status {
						current.status = ""
						break
					}
				}
			}
			for _, attr := range node.attrs {
				add(nodePath()+"/@"+xmlName(attr.Name), attr.Value)
			}
			if len(node.parts) == 0 {
				add(nodePath()+"/text()[1]", "")
			}
		}
		if current.partIndex == len(node.parts) {
			frames = frames[:len(frames)-1]
			continue
		}
		part := node.parts[current.partIndex]
		current.partIndex++
		if part.child != nil {
			next := frame{node: part.child, componentType: current.componentType, componentID: current.componentID, status: current.status}
			frames = append(frames, next)
			continue
		}
		current.textOrdinal++
		// Formatting-only parent whitespace remains in raw XML. Leaf whitespace
		// and mixed-content values retain their exact order and text.
		if current.children == 0 || strings.TrimSpace(part.text) != "" {
			add(nodePath()+"/text()["+strconv.Itoa(current.textOrdinal)+"]", part.text)
		}
	}
	return properties, nil
}

func isComponent(name string) bool {
	switch strings.ToLower(name) {
	case "cell", "lun", "physicaldisk", "celldisk", "griddisk", "flashcache", "flashlog", "iormplan", "iormobject", "offloadgroup", "offloadserver":
		return true
	default:
		return false
	}
}
func nodeText(node *xmlNode) string {
	var value strings.Builder
	for _, part := range node.parts {
		if part.child == nil {
			value.WriteString(part.text)
		}
	}
	return value.String()
}
func componentName(node *xmlNode) string {
	for _, field := range []string{"name", "id"} {
		var names []string
		for _, attr := range node.attrs {
			if attr.Name.Space == "" && strings.EqualFold(attr.Name.Local, field) && strings.TrimSpace(attr.Value) != "" {
				names = append(names, strings.TrimSpace(attr.Value))
			}
		}
		for _, part := range node.parts {
			if part.child != nil && part.child.name.Space == node.name.Space && strings.EqualFold(part.child.name.Local, field) && strings.TrimSpace(nodeText(part.child)) != "" {
				names = append(names, strings.TrimSpace(nodeText(part.child)))
			}
		}
		if len(names) > 0 {
			for _, name := range names[1:] {
				if name != names[0] {
					return ""
				}
			}
			return names[0]
		}
	}
	return ""
}
