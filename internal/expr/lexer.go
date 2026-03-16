package expr

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType represents the type of a lexer token.
type TokenType int

const (
	TokenIdent    TokenType = iota // identifier
	TokenDot                       // .
	TokenString                    // 'string'
	TokenNumber                    // 123, 1.5
	TokenBool                      // true, false
	TokenNull                      // null
	TokenLParen                    // (
	TokenRParen                    // )
	TokenComma                     // ,
	TokenEq                        // ==
	TokenNeq                       // !=
	TokenLt                        // <
	TokenGt                        // >
	TokenLe                        // <=
	TokenGe                        // >=
	TokenAnd                       // &&
	TokenOr                        // ||
	TokenNot                       // !
	TokenStar                      // *
	TokenLBracket                  // [
	TokenRBracket                  // ]
	TokenEOF
)

// Token represents a single lexer token.
type Token struct {
	Type  TokenType
	Value string
}

func (t Token) String() string {
	return fmt.Sprintf("{%d %q}", t.Type, t.Value)
}

// Lex tokenizes the input string (the contents inside ${{ }}).
func Lex(input string) ([]Token, error) {
	var tokens []Token
	i := 0
	for i < len(input) {
		ch := input[i]

		// Skip whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}

		switch {
		case ch == '.':
			tokens = append(tokens, Token{TokenDot, "."})
			i++
		case ch == '(':
			tokens = append(tokens, Token{TokenLParen, "("})
			i++
		case ch == ')':
			tokens = append(tokens, Token{TokenRParen, ")"})
			i++
		case ch == ',':
			tokens = append(tokens, Token{TokenComma, ","})
			i++
		case ch == '[':
			tokens = append(tokens, Token{TokenLBracket, "["})
			i++
		case ch == ']':
			tokens = append(tokens, Token{TokenRBracket, "]"})
			i++
		case ch == '*':
			tokens = append(tokens, Token{TokenStar, "*"})
			i++
		case ch == '=' && i+1 < len(input) && input[i+1] == '=':
			tokens = append(tokens, Token{TokenEq, "=="})
			i += 2
		case ch == '!' && i+1 < len(input) && input[i+1] == '=':
			tokens = append(tokens, Token{TokenNeq, "!="})
			i += 2
		case ch == '!':
			tokens = append(tokens, Token{TokenNot, "!"})
			i++
		case ch == '<' && i+1 < len(input) && input[i+1] == '=':
			tokens = append(tokens, Token{TokenLe, "<="})
			i += 2
		case ch == '<':
			tokens = append(tokens, Token{TokenLt, "<"})
			i++
		case ch == '>' && i+1 < len(input) && input[i+1] == '=':
			tokens = append(tokens, Token{TokenGe, ">="})
			i += 2
		case ch == '>':
			tokens = append(tokens, Token{TokenGt, ">"})
			i++
		case ch == '&' && i+1 < len(input) && input[i+1] == '&':
			tokens = append(tokens, Token{TokenAnd, "&&"})
			i += 2
		case ch == '|' && i+1 < len(input) && input[i+1] == '|':
			tokens = append(tokens, Token{TokenOr, "||"})
			i += 2
		case ch == '\'':
			// String literal
			end := strings.IndexByte(input[i+1:], '\'')
			if end == -1 {
				return nil, fmt.Errorf("unterminated string at position %d", i)
			}
			val := input[i+1 : i+1+end]
			tokens = append(tokens, Token{TokenString, val})
			i = i + 1 + end + 1
		case ch >= '0' && ch <= '9':
			j := i
			hasDot := false
			for j < len(input) && (input[j] >= '0' && input[j] <= '9' || input[j] == '.' && !hasDot) {
				if input[j] == '.' {
					hasDot = true
				}
				j++
			}
			tokens = append(tokens, Token{TokenNumber, input[i:j]})
			i = j
		case ch == '-' && i+1 < len(input) && input[i+1] >= '0' && input[i+1] <= '9':
			j := i + 1
			hasDot := false
			for j < len(input) && (input[j] >= '0' && input[j] <= '9' || input[j] == '.' && !hasDot) {
				if input[j] == '.' {
					hasDot = true
				}
				j++
			}
			tokens = append(tokens, Token{TokenNumber, input[i:j]})
			i = j
		case isIdentStart(rune(ch)):
			j := i
			for j < len(input) && isIdentPart(rune(input[j])) {
				j++
			}
			word := input[i:j]
			switch word {
			case "true", "false":
				tokens = append(tokens, Token{TokenBool, word})
			case "null":
				tokens = append(tokens, Token{TokenNull, word})
			default:
				tokens = append(tokens, Token{TokenIdent, word})
			}
			i = j
		default:
			return nil, fmt.Errorf("unexpected character %q at position %d", ch, i)
		}
	}
	tokens = append(tokens, Token{TokenEOF, ""})
	return tokens, nil
}

func isIdentStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

func isIdentPart(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-'
}
