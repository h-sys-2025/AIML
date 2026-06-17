package main

import (
	"fmt"
	"strings"
	"unicode"
)

// Token types
type TokenType string

const (
	TOKEN_TAG_OPEN  TokenType = "TAG_OPEN"
	TOKEN_TAG_CLOSE TokenType = "TAG_CLOSE"
	TOKEN_ATTR_KEY  TokenType = "ATTR_KEY"
	TOKEN_ATTR_VAL  TokenType = "ATTR_VAL"
	TOKEN_TEXT      TokenType = "TEXT"
	TOKEN_EOF       TokenType = "EOF"
)

type Token struct {
	Type  TokenType
	Value string
}

// AST nodes
type Node struct {
	Tag      string
	Attrs    map[string]string
	Text     string
	Children []*Node
	IsClose  bool
}

// Lexer tokenizes the AI markup language
// Format:
//   <tool:name arg1="val" arg2="val">content</tool:name>
//   <think>reasoning</think>
//   <answer>final answer</answer>
//   <loop>keep going</loop>
//   Plain text is treated as answer text.

type Lexer struct {
	src  string
	pos  int
	runes []rune
}

func NewLexer(src string) *Lexer {
	return &Lexer{src: src, runes: []rune(src)}
}

func (l *Lexer) peek() (rune, bool) {
	if l.pos >= len(l.runes) {
		return 0, false
	}
	return l.runes[l.pos], true
}

func (l *Lexer) next() (rune, bool) {
	if l.pos >= len(l.runes) {
		return 0, false
	}
	r := l.runes[l.pos]
	l.pos++
	return r, true
}

func (l *Lexer) skipWS() {
	for {
		r, ok := l.peek()
		if !ok || !unicode.IsSpace(r) {
			break
		}
		l.next()
	}
}

func (l *Lexer) readUntil(stop rune) string {
	var sb strings.Builder
	for {
		r, ok := l.peek()
		if !ok || r == stop {
			break
		}
		l.next()
		sb.WriteRune(r)
	}
	return sb.String()
}

func (l *Lexer) readUntilString(stop string) string {
	stopRunes := []rune(stop)
	var sb strings.Builder
	for {
		if l.pos+len(stopRunes) > len(l.runes) {
			// consume rest
			for l.pos < len(l.runes) {
				sb.WriteRune(l.runes[l.pos])
				l.pos++
			}
			break
		}
		match := true
		for i, sr := range stopRunes {
			if l.runes[l.pos+i] != sr {
				match = false
				break
			}
		}
		if match {
			break
		}
		r, _ := l.next()
		sb.WriteRune(r)
	}
	return sb.String()
}

// ParseBlocks parses AI output into a sequence of Nodes
func ParseBlocks(src string) ([]*Node, error) {
	src = strings.TrimSpace(src)
	var nodes []*Node
	l := NewLexer(src)

	for l.pos < len(l.runes) {
		r, ok := l.peek()
		if !ok {
			break
		}

		if r == '<' {
			node, err := parseTag(l)
			if err != nil {
				// Not a valid tag, treat as text
				nodes = append(nodes, &Node{Tag: "text", Text: string(r)})
				l.next()
				continue
			}
			if node != nil {
				nodes = append(nodes, node)
			}
		} else {
			// Collect plain text until next tag
			var sb strings.Builder
			for {
				rr, ok2 := l.peek()
				if !ok2 || rr == '<' {
					break
				}
				l.next()
				sb.WriteRune(rr)
			}
			txt := strings.TrimSpace(sb.String())
			if txt != "" {
				nodes = append(nodes, &Node{Tag: "text", Text: txt})
			}
		}
	}
	return nodes, nil
}

func parseTag(l *Lexer) (*Node, error) {
	start := l.pos
	l.next() // consume '<'

	r, ok := l.peek()
	if !ok {
		return nil, fmt.Errorf("unexpected EOF after <")
	}

	isClose := false
	if r == '/' {
		isClose = true
		l.next()
	}

	// Read tag name (can include : like tool:read_file)
	var nameSB strings.Builder
	for {
		rr, ok2 := l.peek()
		if !ok2 {
			break
		}
		if unicode.IsLetter(rr) || unicode.IsDigit(rr) || rr == ':' || rr == '_' || rr == '-' {
			l.next()
			nameSB.WriteRune(rr)
		} else {
			break
		}
	}
	name := nameSB.String()
	if name == "" {
		// Not a tag we understand, reset
		l.pos = start
		return nil, fmt.Errorf("not a tag")
	}

	attrs := map[string]string{}

	if !isClose {
		// Parse attributes
		for {
			l.skipWS()
			rr, ok2 := l.peek()
			if !ok2 || rr == '>' || rr == '/' {
				break
			}
			// read key
			var keySB strings.Builder
			for {
				rrr, ok3 := l.peek()
				if !ok3 || rrr == '=' || rrr == '>' || unicode.IsSpace(rrr) {
					break
				}
				l.next()
				keySB.WriteRune(rrr)
			}
			key := keySB.String()
			l.skipWS()
			rr, _ = l.peek()
			if rr == '=' {
				l.next() // consume =
				l.skipWS()
				rr, _ = l.peek()
				var val string
				if rr == '"' || rr == '\'' {
					quote := rr
					l.next()
					val = l.readUntil(quote)
					l.next() // consume closing quote
				} else {
					val = l.readUntil('>')
				}
				attrs[key] = val
			} else if key != "" {
				attrs[key] = "true"
			}
		}
	}

	// consume '>'
	rr, ok2 := l.peek()
	if !ok2 {
		return nil, fmt.Errorf("unclosed tag")
	}
	if rr == '/' {
		l.next()
	}
	rr, ok2 = l.peek()
	if ok2 && rr == '>' {
		l.next()
	}

	if isClose {
		return &Node{Tag: name, IsClose: true}, nil
	}

	// Read inner content until </name>
	closeTag := "</" + name + ">"
	inner := l.readUntilString(closeTag)
	// consume close tag
	for i := 0; i < len([]rune(closeTag)); i++ {
		l.next()
	}

	return &Node{
		Tag:   name,
		Attrs: attrs,
		Text:  strings.TrimSpace(inner),
	}, nil
}