package parser

import (
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cozees/cook/pkg/cook/token"
)

const (
	bom = 0xFEFF // byte order mark, only permitted as very first character
	eof = -1
)

type scanMode int

const (
	scanNormal scanMode = 1 << iota
	scanArgument
	scanStringITP
	scanAllowExpr
)

func (s scanMode) isMode(m scanMode) bool { return s&m == m }

type scanner struct {
	src  []byte
	file *token.File

	ch         rune
	offset     int
	rdOffset   int
	lineOffset int

	stringTerminate rune
	mode            scanMode
	prevTok         [2]token.Token
	skipLineFeed    bool
	errorHandler    ErrorHandler
}

type ErrorHandler func(p token.Position, msg string, args ...any)

func NewScanner(file *token.File, eh ErrorHandler) (*scanner, error) {
	src, err := os.ReadFile(file.Name())
	if err != nil {
		return nil, err
	}
	return NewScannerSrc(file, src, eh)
}

func NewScannerSrc(file *token.File, src []byte, eh ErrorHandler) (*scanner, error) {
	s := &scanner{
		src:          src,
		file:         file,
		ch:           eof,
		offset:       -1,
		rdOffset:     0,
		lineOffset:   0,
		skipLineFeed: false,
		errorHandler: eh,
		mode:         scanNormal,
	}
	s.next()
	return s, nil
}

func (s *scanner) next() {
	if s.rdOffset < len(s.src) {
		if s.ch == '\n' {
			s.lineOffset = s.offset
			s.file.AddLine(s.offset)
		}
		s.offset = s.rdOffset
		r, w := rune(s.src[s.rdOffset]), 1
		switch {
		case r == 0:
			s.errorHandler(s.file.Position(s.offset), "illegal character NUL")
		case r >= utf8.RuneSelf:
			// not ASCII
			r, w = utf8.DecodeRune(s.src[s.rdOffset:])
			if r == utf8.RuneError && w == 1 {
				s.errorHandler(s.file.Position(s.offset), "illegal UTF-8 encoding")
			} else if r == bom && s.offset > 0 {
				s.errorHandler(s.file.Position(s.offset), "illegal byte order mark")
			}
		case r == '\r':
			// handle \r, \r\n and \n
			nOffs := s.rdOffset + w
			// shift next if it was newline
			if nOffs < len(s.src) && s.src[nOffs] == '\n' {
				s.rdOffset = nOffs
			}
			// treat \r as \n regardless whether line feed is \r or \r\n
			// so subsequence check only need to check \n
			r = '\n'
		}
		s.rdOffset += w
		s.ch = r
	} else {
		s.offset = len(s.src)
		if s.ch == '\n' {
			s.lineOffset = s.offset
			s.file.AddLine(s.offset)
		}
		s.ch = eof
	}
}

func (s *scanner) peek() byte {
	if s.rdOffset < len(s.src) {
		return s.src[s.rdOffset]
	}
	return 0
}

func (s *scanner) skipWhitespace() {
	for (isSpace(s.ch) || (s.skipLineFeed && isLineFeed(s.ch))) && s.ch != eof {
		s.next()
	}
}

func (s *scanner) Scan() (offset int, tok token.Token, lit string) {
	skipLineFeed := true
	// track previous token
	defer func() { s.prevTok[0], s.prevTok[1] = s.prevTok[1], tok }()
	if s.mode&scanArgument != scanArgument {
		if (s.prevTok[0] == token.AT || s.prevTok[0] == token.HASH) && s.prevTok[1] == token.IDENT {
			s.mode |= scanArgument
		}
	}
revisit:
	offs := s.offset
	s.skipWhitespace()
	switch ch := s.ch; {
	case ch == eof:
		offset = s.offset
		if !s.skipLineFeed {
			tok, lit = token.LF, "\n"
			s.skipLineFeed = true
			s.mode &^= scanArgument
			return
		}
		tok = token.EOF
		return
	case isLineFeed(ch):
		// if we arrive here meaning line feed is required and skipLineFeed is set to false
		offset, tok, lit = s.offset, token.LF, "\n"
		s.skipLineFeed = true
		s.mode &^= scanArgument
		return
	case isLetter(ch):
		if s.mode.isMode(scanStringITP) && s.prevTok[1] != token.VAR && !s.mode.isMode(scanAllowExpr) {
			tok, lit = s.scanString(offs, s.stringTerminate)
			offset = offs
			skipLineFeed = false
		} else {
			offset = s.offset
			tok, lit = s.scanIdentifier()
			if !s.mode.isMode(scanStringITP) {
				if len(lit) > 1 {
					tok = token.Lookup(lit, tok)
					if lit == "false" || lit == "true" {
						skipLineFeed = false
						tok = token.BOOLEAN
					} else {
						switch tok {
						case token.IDENT, token.BREAK, token.CONTINUE, token.RETURN:
							skipLineFeed = false
						}
					}
				} else {
					skipLineFeed = false
				}
			} else if tok == token.EXISTS {
				skipLineFeed = true
				s.mode &^= scanArgument
			}
		}
	case isDecimal(ch):
		offset = s.offset
		tok, lit = s.scanNumber()
		skipLineFeed = false
	default:
		// if interpolation has not begin with { then only identifier is allow
		// expr in interpolation must wrapped between ${ expr... }
		if s.mode.isMode(scanStringITP) && !s.mode.isMode(scanAllowExpr) && s.ch != '{' && s.ch != '$' && s.prevTok[1] != token.VAR {
			if s.stringTerminate != 0 {
				tok, lit = s.scanString(offs, s.stringTerminate)
				offset = offs
			} else {
				offset = s.offset
				tok, lit = s.scanString(s.offset, s.stringTerminate)
			}
			s.skipLineFeed = false
			return
		}

		offset = s.offset
		s.next()
		switch ch {
		case '\\':
			if isLineFeed(s.ch) && s.mode&scanArgument == scanArgument {
				// multiple line command or call function argument skip the newline
				s.next()
				goto revisit
			}
		case '\'', '"', '`':
			// string
			offset = s.offset
			tok, lit = s.scanString(s.offset, ch)
			if tok == token.STRING_ITP {
				s.stringTerminate = ch
			} else {
				s.skipLineFeed = false
			}
			return
		case '@':
			tok = token.AT
		case '#':
			tok = token.HASH
		case '~':
			tok = token.FD
		case '$':
			tok = token.VAR
		case '?':
			tok = s.ternary(s.ch == '?', token.DQS, token.QES)
		case '!':
			tok = s.ternary(s.ch == '=', token.NEQ, token.NOT)
		case '^':
			tok = token.XOR
		case '&':
			tok = s.ternary(s.ch == '&', token.LAND, s.ternary(s.ch == '^', token.AND_NOT, token.AND))
		case '%':
			tok = s.ternary(s.ch == '=', token.REM_ASSIGN, token.REM)
		case '|':
			if s.mode.isMode(scanArgument) {
				tok = token.PIPE
			} else {
				tok = s.ternary(s.ch == '|', token.LOR, s.ternary(s.ch == '=', token.OR_ASSIGN, token.OR))
			}
		case '=':
			tok = s.ternary(s.ch == '=', token.EQL, s.ternary(s.ch == '>', token.LAMBDA, token.ASSIGN))
		case '+':
			tok = s.ternary(s.ch == '=', token.ADD_ASSIGN, s.ternary(s.ch == '+', token.INC, token.ADD))
			if tok == token.INC {
				skipLineFeed = false
			}
		case '-':
			tok = s.ternary(s.ch == '=', token.SUB_ASSIGN, s.ternary(s.ch == '-', token.DEC, token.SUB))
			if tok == token.DEC {
				skipLineFeed = false
			}
		case '/':
			if s.ch == '/' || s.ch == '*' {
				tok, lit = token.COMMENT, s.scanComment()
				skipLineFeed = true
			} else {
				tok = s.ternary(s.ch == '=', token.QUO_ASSIGN, token.QUO)
			}
		case '*':
			tok = s.ternary(s.ch == '=', token.MUL_ASSIGN, token.MUL)
		case ':':
			tok = token.COLON
		case ',':
			tok = token.COMMA
		case '.':
			if s.ch != '.' {
				s.errorHandler(s.file.Position(s.offset), "invalid symbol .")
				return
			}
			s.next()
			tok = token.RANGE
			lit = ".."
			skipLineFeed = true
		case '[':
			tok = token.LBRACK
			skipLineFeed = true
		case ']':
			tok = token.RBRACK
			skipLineFeed = false
		case '{':
			tok = token.LBRACE
			skipLineFeed = true
			if s.mode.isMode(scanStringITP) {
				s.mode |= scanAllowExpr
			}
		case '}':
			tok = token.RBRACE
			skipLineFeed = false
			if s.mode.isMode(scanStringITP) {
				s.mode &^= scanAllowExpr
			}
		case '(':
			tok = token.LPAREN
			skipLineFeed = true
		case ')':
			tok = token.RPAREN
			skipLineFeed = false
		case '>':
			if s.mode&scanArgument == scanArgument {
				tok = s.ternary(s.ch == '>', token.APPEND_TO, token.WRITE_TO)
			} else {
				tok = s.ternary(s.ch == '=', token.GEQ, s.ternary(s.ch == '>', token.SHR, token.GTR))
			}
		case '<':
			if s.mode&scanArgument == scanArgument {
				tok = token.READ_FROM
			} else {
				tok = s.ternary(s.ch == '=', token.LEQ, s.ternary(s.ch == '<', token.SHL, token.LSS))
			}
		case '≥':
			tok = token.GEQ
		case '≤':
			tok = token.LEQ
		case '≠':
			tok = token.NEQ
		}
		lit = string(s.src[offset:s.offset])
	}
	s.skipLineFeed = skipLineFeed
	return
}

func (s *scanner) scanComment() string {
	offs := s.offset - 1
	if s.ch == '/' {
		for s.next(); s.ch != '\n' && s.ch != eof; s.next() {
		}
		return string(s.src[offs:s.offset])
	} else {
		s.next()
		for {
			ch := s.ch
			s.next()
			if ch == '*' && s.ch == '/' {
				s.next()
				break
			} else if ch == eof {
				s.errorHandler(s.file.Position(offs), "comment not terminated")
				return ""
			}
		}
		return string(s.src[offs:s.offset])
	}
}

func (s *scanner) scanNumber() (token.Token, string) {
	offs := s.offset
	tok := token.ILLEGAL
	// integer part
	if s.ch != '.' {
		tok = token.INTEGER
		if tok, lit := s.scanDigitOrString(offs, false); tok == token.STRING || tok == token.STRING_ITP {
			return tok, lit
		}
	}
	// fractional part
	if s.ch == '.' {
		if s.peek() == '.' {
			return tok, string(s.src[offs:s.offset])
		}
		tok = token.FLOAT
		s.next()
		if tok, lit := s.scanDigitOrString(offs, true); tok == token.STRING || tok == token.STRING_ITP {
			return tok, lit
		}
	}
	return tok, string(s.src[offs:s.offset])
}

func (s *scanner) scanDigitOrString(offs int, ignoreDot bool) (tok token.Token, lit string) {
	var rdOffset int
	var b byte
	for _, b = range s.src[s.rdOffset:] {
		if '0' <= b && b <= '9' {
			rdOffset++
			continue
		}
		break
	}
	s.rdOffset += rdOffset
	s.offset = s.rdOffset
	if s.rdOffset < len(s.src) {
		s.ch = rune(b)
		s.rdOffset++
	} else {
		s.ch = -1
	}
	return
}

func (s *scanner) scanIdentifier() (tok token.Token, lit string) {
	identOffs := s.offset
	if s.rdOffset == len(s.src) {
		s.next()
	} else {
		for rdOffset, b := range s.src[s.rdOffset:] {
			if 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || b == '_' || '0' <= b && b <= '9' {
				continue
			}
			s.rdOffset += rdOffset
			if 0 < b && b < utf8.RuneSelf {
				s.ch = rune(b)
				s.offset = s.rdOffset
				s.rdOffset++
				goto exit
			}
			s.next()
			for isLetter(s.ch) || isDigit(s.ch) {
				s.next()
			}
			goto exit
		}
		// reaching end of file
		s.offset = len(s.src)
		s.ch = -1
		s.rdOffset = s.offset
	}
exit:
	return token.IDENT, string(s.src[identOffs:s.offset])
}

func (s *scanner) scanString(offset int, terminate rune) (tok token.Token, lit string) {
	if terminate == '`' {
		return s.scanRawString(offset)
	}
	tok = token.STRING
	if s.mode.isMode(scanStringITP) {
		tok = token.STRING_ITP
	}
	for {
		ch := s.ch
		if (ch == '\n' && terminate != '`') || ch == eof {
			if terminate != 0 {
				s.errorHandler(s.file.Position(offset), "string literal not terminated")
			}
			break
		} else if ch == '$' {
			tok, lit = token.STRING_ITP, string(s.src[offset:s.offset])
			s.mode |= scanStringITP
			return
		}
		s.next()
		if ch == terminate {
			s.mode &^= scanAllowExpr | scanStringITP
			break
		}
		if ch == '\\' {
			s.scanEscape(terminate)
		}
	}
	if terminate != 0 {
		return tok, string(s.src[offset : s.offset-1])
	} else {
		return tok, string(s.src[offset:s.offset])
	}
}

func (s *scanner) scanRawString(offset int) (tok token.Token, lit string) {
	builder := &strings.Builder{}
	builder.Write(s.src[offset:s.offset])
	tok = token.STRING
	if s.mode.isMode(scanStringITP) {
		tok = token.STRING_ITP
	}
	for {
		ch := s.ch
		if ch < 0 {
			s.errorHandler(s.file.Position(offset), "raw string literal not terminated")
			break
		}
		if ch == '$' {
			tok = token.STRING_ITP
			s.stringTerminate = '`'
			s.mode = scanStringITP
			break
		}
		s.next()
		if ch == '`' {
			break
		}
		builder.WriteRune(ch)
	}
	lit = builder.String()
	return
}

func (s *scanner) scanEscape(terminate rune) bool {
	offs := s.offset

	switch s.ch {
	case '\'', '"', '`':
		if s.ch == terminate {
			s.next()
			return true
		}
	case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', '$':
		s.next()
		return true
	default:
		msg := "unknown escape sequence"
		if s.ch < 0 {
			msg = "escape sequence not terminated"
		}
		s.errorHandler(s.file.Position(offs), msg)
		return false
	}
	return true
}

func (s *scanner) ternary(cond bool, t, f token.Token) token.Token {
	if cond {
		s.next()
		return t
	} else {
		return f
	}
}

func lower(ch rune) rune     { return ('a' - 'A') | ch } // returns lower-case ch iff ch is ASCII letter
func isDecimal(ch rune) bool { return '0' <= ch && ch <= '9' }

func isLetter(ch rune) bool {
	return 'a' <= lower(ch) && lower(ch) <= 'z' || ch == '_' || ch >= utf8.RuneSelf && unicode.IsLetter(ch)
}

func isDigit(ch rune) bool {
	return isDecimal(ch) || ch >= utf8.RuneSelf && unicode.IsDigit(ch)
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' }

func isLineFeed(r rune) bool { return r == '\n' || r == '\r' }
