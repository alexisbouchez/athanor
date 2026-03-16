package expr

import (
	"fmt"
	"strconv"
)

// Node is the interface for all AST nodes.
type Node interface {
	exprNode()
}

// Literal represents a constant value (string, float64, bool, nil).
type Literal struct {
	Value any
}

// Context represents a dotted context access like github.sha → ["github","sha"].
type Context struct {
	Parts []string
}

// Index represents bracket access like matrix['os'].
type Index struct {
	Object Node
	Key    Node
}

// FuncCall represents a function call like contains(x, y).
type FuncCall struct {
	Name string
	Args []Node
}

// BinaryOp represents a binary operation like a == b.
type BinaryOp struct {
	Op    string
	Left  Node
	Right Node
}

// UnaryOp represents a unary operation like !x.
type UnaryOp struct {
	Op      string
	Operand Node
}

func (Literal) exprNode()  {}
func (Context) exprNode()  {}
func (Index) exprNode()    {}
func (FuncCall) exprNode() {}
func (BinaryOp) exprNode() {}
func (UnaryOp) exprNode()  {}

// Parser is a recursive descent parser for GitHub Actions expressions.
type Parser struct {
	tokens []Token
	pos    int
}

// Parse parses the token stream into an AST.
func Parse(tokens []Token) (Node, error) {
	p := &Parser{tokens: tokens}
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().Type != TokenEOF {
		return nil, fmt.Errorf("unexpected token %v after expression", p.peek())
	}
	return node, nil
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	t := p.peek()
	p.pos++
	return t
}

func (p *Parser) expect(tt TokenType) (Token, error) {
	t := p.advance()
	if t.Type != tt {
		return t, fmt.Errorf("expected token type %d, got %v", tt, t)
	}
	return t, nil
}

// Precedence: || < && < ==,!= < <,>,<=,>= < ! (unary) < primary

func (p *Parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = BinaryOp{Op: "||", Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseAnd() (Node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenAnd {
		p.advance()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = BinaryOp{Op: "&&", Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseEquality() (Node, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenEq || p.peek().Type == TokenNeq {
		op := p.advance().Value
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = BinaryOp{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseComparison() (Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenLt || p.peek().Type == TokenGt || p.peek().Type == TokenLe || p.peek().Type == TokenGe {
		op := p.advance().Value
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = BinaryOp{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseUnary() (Node, error) {
	if p.peek().Type == TokenNot {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return UnaryOp{Op: "!", Operand: operand}, nil
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() (Node, error) {
	node, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().Type {
		case TokenDot:
			p.advance()
			t := p.advance()
			if t.Type != TokenIdent && t.Type != TokenStar {
				return nil, fmt.Errorf("expected identifier or * after '.', got %v", t)
			}
			// Flatten into Context node if possible
			if ctx, ok := node.(Context); ok {
				ctx.Parts = append(ctx.Parts, t.Value)
				node = ctx
			} else {
				return nil, fmt.Errorf("dot access on non-context node")
			}
		case TokenLBracket:
			p.advance()
			key, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(TokenRBracket); err != nil {
				return nil, err
			}
			node = Index{Object: node, Key: key}
		default:
			return node, nil
		}
	}
}

func (p *Parser) parsePrimary() (Node, error) {
	t := p.peek()
	switch t.Type {
	case TokenString:
		p.advance()
		return Literal{Value: t.Value}, nil
	case TokenNumber:
		p.advance()
		f, err := strconv.ParseFloat(t.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", t.Value, err)
		}
		return Literal{Value: f}, nil
	case TokenBool:
		p.advance()
		return Literal{Value: t.Value == "true"}, nil
	case TokenNull:
		p.advance()
		return Literal{Value: nil}, nil
	case TokenLParen:
		p.advance()
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return node, nil
	case TokenIdent:
		p.advance()
		name := t.Value
		// Check if it's a function call
		if p.peek().Type == TokenLParen {
			p.advance()
			var args []Node
			if p.peek().Type != TokenRParen {
				arg, err := p.parseOr()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				for p.peek().Type == TokenComma {
					p.advance()
					arg, err := p.parseOr()
					if err != nil {
						return nil, err
					}
					args = append(args, arg)
				}
			}
			if _, err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			return FuncCall{Name: name, Args: args}, nil
		}
		// It's a context access: start of a dotted path
		return Context{Parts: []string{name}}, nil
	default:
		return nil, fmt.Errorf("unexpected token %v", t)
	}
}
