package layout

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

// Marshal returns a compact one-line representation of root. Grammar:
//
//	tree    := leaf | split
//	leaf    := "leaf:profile=" + ident + ["," "title=" + qstring]
//	split   := ("split-v"|"split-h") ":" + ratio + "{" tree "}" "{" tree "}"
//	ident   := word (no spaces, no braces)
//	qstring := "..." (backslash-escaped)
//	ratio   := decimal in (0,1)
//
// "split-v" produces a SplitVertical orientation (panes side-by-side, à la
// SplitH naming convention); "split-h" produces SplitHorizontal (panes
// stacked).
//
// The grammar is intentionally small for v1; sub-step deferred-to-v2
// replaces it with a richer PEG.
func Marshal(root *PaneNode) string {
	if root == nil {
		return ""
	}
	var b strings.Builder
	marshalNode(root, &b)
	return b.String()
}

func marshalNode(n *PaneNode, b *strings.Builder) {
	switch n.Kind {
	case NodeLeaf:
		b.WriteString("leaf:")
		if n.Pane != nil && n.Pane.Profile != "" {
			b.WriteString("profile=")
			b.WriteString(n.Pane.Profile)
		} else {
			b.WriteString("profile=shell")
		}
		if n.Pane != nil && n.Pane.Title != "" {
			b.WriteString(",title=")
			b.WriteString(quote(n.Pane.Title))
		}
	case NodeSplit:
		if n.Orientation == views.SplitVertical {
			b.WriteString("split-v:")
		} else {
			b.WriteString("split-h:")
		}
		b.WriteString(strconv.FormatFloat(n.Ratio, 'f', 3, 64))
		b.WriteByte('{')
		marshalNode(n.A, b)
		b.WriteByte('}')
		b.WriteByte('{')
		marshalNode(n.B, b)
		b.WriteByte('}')
	}
}

// LeafSpec describes what a "leaf:..." node should produce: a profile
// name plus optional title.
type LeafSpec struct {
	Profile string
	Title   string
}

// Unmarshal parses spec into a tree. Each leaf is built via spawnLeaf,
// which is responsible for creating a live session.Pane (typically by
// looking up the profile and calling profile.Instantiate).
func Unmarshal(spec string, spawnLeaf func(LeafSpec) (*session.Pane, error)) (*PaneNode, error) {
	p := &parser{src: spec, pos: 0, spawn: spawnLeaf}
	n, err := p.parse()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("layout: trailing input at offset %d: %q", p.pos, p.src[p.pos:])
	}
	return n, nil
}

type parser struct {
	src   string
	pos   int
	spawn func(LeafSpec) (*session.Pane, error)
}

func (p *parser) parse() (*PaneNode, error) {
	if p.startsWith("leaf:") {
		return p.parseLeaf()
	}
	if p.startsWith("split-v:") || p.startsWith("split-h:") {
		return p.parseSplit()
	}
	return nil, fmt.Errorf("layout: expected leaf/split-v/split-h at offset %d", p.pos)
}

func (p *parser) parseLeaf() (*PaneNode, error) {
	p.pos += len("leaf:")
	spec := LeafSpec{}
	for {
		key, err := p.readUntil(func(r byte) bool { return r == '=' })
		if err != nil {
			return nil, err
		}
		p.pos++ // consume '='
		var value string
		if p.peek() == '"' {
			value, err = p.readQString()
		} else {
			value, err = p.readUntil(func(r byte) bool { return r == ',' || r == '}' || r == 0 })
		}
		if err != nil {
			return nil, err
		}
		switch key {
		case "profile":
			spec.Profile = value
		case "title":
			spec.Title = value
		}
		if p.peek() == ',' {
			p.pos++
			continue
		}
		break
	}
	pane, err := p.spawn(spec)
	if err != nil {
		return nil, err
	}
	return Leaf(pane), nil
}

func (p *parser) parseSplit() (*PaneNode, error) {
	var orient views.SplitOrientation
	if p.startsWith("split-v:") {
		orient = views.SplitVertical
		p.pos += len("split-v:")
	} else {
		orient = views.SplitHorizontal
		p.pos += len("split-h:")
	}
	ratioStr, err := p.readUntil(func(r byte) bool { return r == '{' })
	if err != nil {
		return nil, err
	}
	ratio, err := strconv.ParseFloat(ratioStr, 64)
	if err != nil {
		return nil, fmt.Errorf("layout: bad ratio %q: %w", ratioStr, err)
	}
	if !(ratio > 0 && ratio < 1) {
		return nil, fmt.Errorf("layout: ratio %f out of (0,1)", ratio)
	}
	if p.peek() != '{' {
		return nil, fmt.Errorf("layout: expected '{' at offset %d", p.pos)
	}
	p.pos++
	a, err := p.parse()
	if err != nil {
		return nil, err
	}
	if p.peek() != '}' {
		return nil, fmt.Errorf("layout: expected '}' at offset %d", p.pos)
	}
	p.pos++
	if p.peek() != '{' {
		return nil, fmt.Errorf("layout: expected '{' at offset %d", p.pos)
	}
	p.pos++
	b, err := p.parse()
	if err != nil {
		return nil, err
	}
	if p.peek() != '}' {
		return nil, fmt.Errorf("layout: expected '}' at offset %d", p.pos)
	}
	p.pos++
	n := Split(orient, a, b)
	n.Ratio = ratio
	return n, nil
}

func (p *parser) startsWith(s string) bool {
	return strings.HasPrefix(p.src[p.pos:], s)
}

func (p *parser) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) readUntil(stop func(byte) bool) (string, error) {
	start := p.pos
	for p.pos < len(p.src) && !stop(p.src[p.pos]) {
		p.pos++
	}
	return p.src[start:p.pos], nil
}

func (p *parser) readQString() (string, error) {
	if p.peek() != '"' {
		return "", fmt.Errorf("layout: expected '\"' at offset %d", p.pos)
	}
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\\' && p.pos+1 < len(p.src) {
			b.WriteByte(p.src[p.pos+1])
			p.pos += 2
			continue
		}
		if c == '"' {
			p.pos++
			return b.String(), nil
		}
		b.WriteByte(c)
		p.pos++
	}
	return "", fmt.Errorf("layout: unterminated quoted string")
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	b.WriteByte('"')
	return b.String()
}
