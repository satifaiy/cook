package parser

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cozees/cook/pkg/cook/ast"
	"github.com/cozees/cook/pkg/cook/token"
	cookErrors "github.com/cozees/cook/pkg/errors"
)

type Parser interface {
	Parse(file string) (ast.Cook, error)
	ParseSrc(file *token.File, src []byte) (ast.Cook, error)
}

func NewParser() Parser {
	return &parser{parsed: make(map[string]*token.File), pending: make(map[string]*token.File)}
}

type parser struct {
	pending map[string]*token.File // Cookfile which is waiting for parsing
	parsed  map[string]*token.File // Cookfile which already parsed

	s     *scanner
	tfile *token.File
	block *ast.BlockStatement

	cook ast.Cook

	// current token
	cOffs int
	cTok  token.Token
	cLit  string

	// ahead token by 1 step
	nOffs int
	nTok  token.Token
	nLit  string

	errs *cookErrors.CookError
}

func (p *parser) curPos() token.Position { return p.tfile.Position(p.cOffs) }

func (p *parser) errorHandler(pos token.Position, msg string, args ...any) {
	if p.errs == nil {
		p.errs = &cookErrors.CookError{}
	}
	p.errs.StackError(fmt.Errorf(pos.String()+" "+msg, args...))
	// when encounter error immedate ignore everything until new statement
	for {
		p.next()
		switch p.cTok {
		case token.IDENT:
			if (token.ADD_ASSIGN <= p.nTok && p.nTok <= token.REM_ASSIGN) ||
				(token.AND_ASSIGN <= p.nTok && p.nTok <= token.ASSIGN) || p.nTok == token.LBRACK {
				// p.nTok == token.LBRACK, is false positive as it not guarantee to be the
				// index assigned statement.
				return
			}
		case token.FOR, token.IF, token.BREAK, token.CONTINUE, token.RETURN, token.EOF, token.COMMENT:
			return
		}
	}
}

func (p *parser) expect(require token.Token) (offs int) {
	if p.cTok != require {
		p.errorHandler(p.curPos(), fmt.Sprintf("expect %s but got %s", require, p.cTok))
		offs = -1
	} else {
		offs = p.cOffs
		p.next()
	}
	return
}

func (p *parser) init(file *token.File, src []byte) (err error) {
	p.tfile = file
	if p.s, err = NewScannerSrc(file, src, p.errorHandler); err == nil {
		p.s.skipLineFeed = true
		p.cOffs, p.cTok, p.cLit = -1, 0, ""
		p.nOffs, p.nTok, p.nLit = p.s.Scan()
	}
	return err
}

func (p *parser) next() {
	p.cOffs, p.cTok, p.cLit = p.nOffs, p.nTok, p.nLit
	if p.nTok != token.EOF {
		p.nOffs, p.nTok, p.nLit = p.s.Scan()
	}
}

func (p *parser) Parse(file string) (ast.Cook, error) {
	stat, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	tfile := token.NewFile(file, int(stat.Size()))
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return p.ParseSrc(tfile, src)
}

func (p *parser) ParseSrc(file *token.File, src []byte) (ast.Cook, error) {
	if err := p.init(file, src); err == nil {
		p.cook = ast.NewCook()
		p.block = p.cook.Block()
		return p.parse()
	} else {
		return nil, err
	}
}

func (p *parser) parse() (cook ast.Cook, err error) {
	// scan include directive first
nextFile:
	for p.next(); p.cTok != token.EOF; {
		if p.cTok == token.INCLUDE {
			p.parseIncludeDirective()
			continue
		}
		break
	}
	for p.cTok != token.EOF {
		switch p.cTok {
		case token.INCLUDE:
			p.errorHandler(p.curPos(), "include directive must place at the very top of the file.")
		case token.IDENT:
			p.parseIdentifier(true)
		case token.FOR:
			p.parseForLoop(false)
		case token.IF:
			p.parseIf(false, nil)
		case token.AT, token.HASH:
			p.parseCallReference(false, nil)
		case token.EXIT:
			offs := p.cOffs
			if code := p.parseBinaryExpr(false, token.LowestPrec+1); code != nil {
				p.block.Append(&ast.ExprWrapperStatement{
					X: &ast.Exit{Base: &ast.Base{Offset: offs, File: p.tfile}, ExitCode: code},
				})
			}
			p.expect(token.LF)
		case token.COMMENT:
			p.next()
			// eat the comment for now.
			// TODO: add comment to file which help when formatting the code
		default:
			p.errorHandler(p.curPos(), "invalid token %s", p.cTok)
		}
	}

	// check if there more file pending to parse
	if len(p.pending) > 0 {
		p.parsed[p.tfile.Name()] = p.tfile
		for k, v := range p.pending {
			if src, err := os.ReadFile(v.Name()); err != nil {
				return nil, err
			} else if err := p.init(v, src); err != nil {
				return nil, err
			}
			p.tfile = v
			delete(p.pending, k)
			break
		}
		p.block = p.cook.Block()
		goto nextFile
	}

	if p.errs != nil {
		// return error stack
		return nil, p.errs
	} else {
		return p.cook, nil
	}
}

func (p *parser) parseIncludeDirective() {
	p.next()
	if p.cTok == token.STRING {
		_, ok1 := p.parsed[p.cLit]
		_, ok2 := p.pending[p.cLit]
		if ok1 || ok2 {
			// file have already be parsed, nothing to do here.
			return
		}
		file := p.cLit
		p.next()
		if p.expect(token.LF) == -1 {
			return
		}
		// new file
		ifile := filepath.Join(filepath.Dir(p.s.file.Name()), file)
		if stat, err := os.Stat(ifile); err != nil {
			if os.IsNotExist(err) {
				p.errorHandler(p.curPos(), "included file %s not found", ifile)
			} else {
				p.errorHandler(p.curPos(), "unable to read included file %s ", ifile)
			}
		} else {
			p.pending[p.cLit] = token.NewFile(ifile, int(stat.Size()))
		}
	} else {
		p.errorHandler(p.curPos(), "include directive expected string")
	}
}

func (p *parser) parseIdentifier(head bool) {
	switch p.nTok {
	case token.COLON:
		p.parseTarget()
	case token.LPAREN:
		// function declaration or calling a function
		p.parseDeclareFunction(false)
	case token.LBRACK:
		// index expression
		if x := p.parseIndexExpression(); x != nil {
			p.parseAssignStatement(x)
		}
	case token.LBRACE:
		// slice or delete
		if head {
			p.next()
			p.errorHandler(p.curPos(), "unexpected %s", p.cTok)
			return
		}
	case token.INC, token.DEC:
		offs, lit := p.cOffs, p.cLit
		p.next()
		p.block.Append(&ast.ExprWrapperStatement{
			X: &ast.IncDec{
				Op: p.cTok,
				X:  &ast.Ident{Base: &ast.Base{Offset: offs, File: p.tfile}, Name: lit},
			},
		})
		p.next()
		p.expect(token.LF)
	default:
		settable := &ast.Ident{Base: &ast.Base{Offset: p.cOffs, File: p.tfile}, Name: p.cLit}
		p.parseAssignStatement(settable)
		if p.cTok == token.LF {
			p.next()
		}
	}
}

func (p *parser) parseTarget() {
	offs, name := p.cOffs, p.cLit
	p.next()
	if t, err := p.cook.AddTarget(&ast.Base{File: p.tfile, Offset: offs}, name); err != nil {
		p.errorHandler(p.curPos(), err.Error())
	} else if p.next(); name == "all" && p.cTok == token.MUL {
		t.SetCallAll()
		p.next()
	} else {
		p.block = t.Insts
	}
}

func (p *parser) parseAssignStatement(settableNode ast.SettableNode) {
	offs := p.cOffs
	p.next()
	op := p.cTok
	switch {
	case p.cTok == token.ASSIGN:
		fallthrough
	case token.ADD_ASSIGN <= p.cTok && p.cTok <= token.REM_ASSIGN:
		fallthrough
	case token.AND_ASSIGN <= p.cTok && p.cTok <= token.AND_NOT_ASSIGN:
		assignStmt := &ast.AssignStatement{
			Base:  &ast.Base{Offset: offs, File: p.tfile},
			Op:    op,
			Ident: settableNode,
		}
		if p.nTok == token.AT || p.nTok == token.HASH {
			p.next()
			assignStmt.Value = p.parseCallReference(true, nil)
		} else {
			assignStmt.Value = p.parseBinaryExpr(false, token.LowestPrec+1)
		}
		if p.cTok == token.LF {
			// newline is option on assign statement
			p.next()
		}
		p.block.Append(assignStmt)
	default:
		p.errorHandler(p.curPos(), "unexpected %s", p.cTok)
	}
}

func (p *parser) parseBinaryExpr(isChaining bool, priority int) ast.Node {
	p.next()
	if p.cTok == token.IDENT {
		if p.nTok == token.LPAREN {
			return p.parseTransformation()
		} else if p.nTok == token.EXISTS {
			offs, lit := p.cOffs, p.cLit
			p.next()
			p.next()
			base := &ast.Base{Offset: offs, File: p.tfile}
			return &ast.Exists{Base: base, X: &ast.Ident{Base: base, Name: lit}}
		}
	}

	x := p.parseUnaryExpr()
	if isChaining && p.cTok.IsComparison() {
		return x
	} else if p.cTok == token.EXISTS {

	}

	var prevNode ast.Node
	for {
		op, oprec := p.cTok, p.cTok.Precedence()
		if oprec < priority || isChaining && op.IsComparison() {
			return x
		}
		// check for chaining comparision
		// <, ≤ (<=), >, ≥ (>=), ≠ (!=), ==, is

		// special case for is, ternary and fallback expression
		if op == token.QES {
			// ternary case or short if
			x = p.parseTernaryExpr(x)
		} else if op == token.DQS {
			// fallback expression
			x = p.parseFallbackExpr(x)
		} else if op == token.IS {
			x = p.parseIsExpr(x)
		} else {
			offs := p.cOffs
			isComp := op.IsComparison()
			y := p.parseBinaryExpr(isChaining || isComp, oprec+1)
			ly := y
			if prevNode != nil {
				y = &ast.Binary{
					Base: &ast.Base{Offset: offs, File: p.tfile},
					Op:   op,
					L:    prevNode,
					R:    y,
				}
				op = token.LAND
				prevNode = nil
			}
			x = &ast.Binary{
				Base: &ast.Base{Offset: offs, File: p.tfile},
				Op:   op,
				L:    x,
				R:    y,
			}
			if isComp && p.cTok.IsComparison() {
				prevNode = ly
			}
		}
	}
}

func (p *parser) parseUnaryExpr() (x ast.Node) {
	switch p.cTok {
	case token.ADD, token.SUB, token.NOT, token.XOR, token.FD:
		offs, op := p.cOffs, p.cTok
		p.next()
		opr, _ := p.parseOperand()
		if p.cTok == token.EXISTS && op == token.FD {
			p.next()
			x = &ast.Exists{
				Base: &ast.Base{Offset: offs, File: p.tfile},
				Op:   op,
				X:    opr,
			}
		} else {
			x = &ast.Unary{
				Base: &ast.Base{Offset: offs, File: p.tfile},
				Op:   op,
				X:    opr,
			}
		}
	case token.SIZEOF:
		offs := p.cOffs
		p.next()
		var opr ast.Node
		if p.cTok == token.FD {
			opr = p.parseUnaryExpr()
		} else {
			opr, _ = p.parseOperand()
		}
		x = &ast.SizeOf{
			Base: &ast.Base{Offset: offs, File: p.tfile},
			X:    opr,
		}
	case token.VAR:
		offs := p.cOffs
		p.next()
		lit := p.cLit
		if p.expect(token.INTEGER) != -1 {
			x = &ast.Ident{Base: &ast.Base{Offset: offs, File: p.tfile}, Name: lit}
		}
	case token.TINTEGER, token.TFLOAT, token.TSTRING, token.TBOOLEAN:
		x = p.parseTypeCaseExpr()
	case token.ON:
		offs := p.cOffs
		p.next()
		switch p.cTok {
		case token.LINUX, token.MACOS, token.WINDOWS:
			x = &ast.OSysCheck{Base: &ast.Base{Offset: offs, File: p.tfile}, OS: p.cTok}
			p.next()
		default:
			p.errorHandler(p.curPos(), "expect operating system keyword got %s (%s)", p.cTok, p.cLit)
			return nil
		}
	case token.HASH, token.AT:
		op := p.cTok
		base := &ast.Base{Offset: p.cOffs, File: p.tfile}
		if p.next(); p.cTok == token.IDENT {
			soffs, tok, lit := p.cOffs, p.cTok, p.cLit
			if p.next(); p.cTok != token.EXISTS {
				p.errorHandler(p.curPos(), "expect %s but got %s", token.EXISTS, p.cTok)
				return nil
			}
			if tok == token.STRING {
				x = &ast.BasicLit{Base: base, Lit: lit, Kind: tok, Mark: p.s.src[soffs-1]}
			} else {
				x = &ast.Ident{Base: base, Name: lit}
			}
		} else {
			p.errorHandler(p.curPos(), "expect identifier or file expression but got %s", p.cTok)
			return nil
		}
		x = &ast.Exists{Base: base, Op: op, X: x}
		p.next()
	default:
		offs := p.cOffs
		x, _ = p.parseOperand()
		if p.cTok == token.EXISTS {
			p.next()
			x = &ast.Exists{Base: &ast.Base{Offset: offs, File: p.tfile}, X: x}
		}
	}
	return
}

func (p *parser) parseOperand() (x ast.Node, kind token.Token) {
	switch p.cTok {
	case token.IDENT:
		if p.nTok == token.LBRACK {
			x = p.parseIndexExpression()
		} else {
			offs := p.cOffs
			x = &ast.Ident{Base: &ast.Base{Offset: offs, File: p.tfile}, Name: p.cLit}
		}
		p.next()
		kind = token.IDENT
	case token.INTEGER, token.FLOAT, token.STRING, token.BOOLEAN:
		if p.cTok == token.STRING {
			x = &ast.BasicLit{Base: &ast.Base{Offset: p.cOffs, File: p.tfile}, Lit: p.cLit, Kind: p.cTok, Mark: p.s.src[p.cOffs-1]}
		} else {
			x = &ast.BasicLit{Base: &ast.Base{Offset: p.cOffs, File: p.tfile}, Lit: p.cLit, Kind: p.cTok}
		}
		kind = p.cTok
		p.next()
	case token.STRING_ITP:
		x, kind = p.parseStringInterpolation(), token.STRING
	case token.LPAREN:
		lparen := p.cOffs
		inx := p.parseBinaryExpr(false, token.LowestPrec+1) // types may be parenthesized: (some type)
		if p.expect(token.RPAREN) == -1 {
			return nil, 0
		}
		kind = token.LPAREN
		x = &ast.Paren{Base: &ast.Base{Offset: lparen, File: p.tfile}, Inner: inx}
	case token.LBRACE:
		x, kind = p.parseMapLiteral(), token.MAP
	case token.LBRACK:
		x, kind = p.parserArrayLiteral(), token.ARRAY
	default:
		p.errorHandler(p.curPos(), fmt.Sprintf("invalid token %s", p.cTok))
	}
	return
}

func (p *parser) parseStringInterpolation() ast.Node {
	offs := p.cOffs
	sib := ast.NewStringInterpolationBuilder(p.s.src[offs-1])
	for {
		switch {
		case p.cTok == token.STRING_ITP:
			sib.WriteString(p.cLit)
			p.next()
			if p.cTok != token.VAR {
				return sib.Build(offs, p.tfile)
			}
			p.next()
		case p.cTok == token.VAR:
			p.next()
		default:
			return sib.Build(offs, p.tfile)
		}
		// require following to a variable or an expression
		switch p.cTok {
		case token.IDENT:
			sib.AddExpression(&ast.Ident{Base: &ast.Base{Offset: p.cOffs, File: p.tfile}, Name: p.cLit})
			p.next()
		case token.LBRACE:
			x := p.parseBinaryExpr(false, token.LowestPrec+1)
			if p.expect(token.RBRACE) == -1 {
				return nil
			}
			sib.AddExpression(x)
		}
	}
}

func (p *parser) parseMapLiteral() ast.Node {
	offs := p.s.offset
	p.next()
	var keys []ast.Node
	var values []ast.Node
	keys = make([]ast.Node, 0)
	values = make([]ast.Node, 0)
	if p.cTok != token.RBRACE {
	loop:
		for {
			k, _ := p.parseOperand()
			keys = append(keys, k)
			if p.expect(token.COLON) != -1 {
				v, _ := p.parseOperand()
				values = append(values, v)
			} else {
				return nil
			}
			switch p.cTok {
			case token.RBRACE, token.EOF:
				break loop
			case token.COMMA:
				p.next()
				if p.cTok == token.RBRACE {
					break loop
				}
			}
		}
	}
	if p.expect(token.RBRACE) != -1 {
		return &ast.MapLiteral{Base: &ast.Base{Offset: offs, File: p.tfile}, Keys: keys, Values: values}
	} else {
		return nil
	}
}

func (p *parser) parserArrayLiteral() ast.Node {
	offs := p.s.offset
	p.next()
	var values []ast.Node
	if p.cTok != token.RBRACK {
		x, tok := p.parseOperand()
		if isGlob, nodes := parseArrayFile(x, tok); isGlob {
			values = append(values, nodes...)
		} else {
			values = append(values, x)
		}
	} else {
		values = make([]ast.Node, 0)
	}
loop:
	for {
		switch p.cTok {
		case token.RBRACK, token.EOF:
			break loop
		case token.COMMA:
			p.next()
			if p.cTok == token.RBRACK {
				break loop
			}
			y, tok := p.parseOperand()
			if isGlob, nodes := parseArrayFile(y, tok); isGlob {
				values = append(values, nodes...)
			} else {
				values = append(values, y)
			}
		}
	}
	if p.expect(token.RBRACK) != -1 {
		return &ast.ArrayLiteral{Base: &ast.Base{Offset: offs, File: p.tfile}, Values: values}
	} else {
		return nil
	}
}

func (p *parser) parseForLoop(inForLoop bool) {
	offs := p.cOffs
	p.next()
	label := ""
	if p.cTok == token.COLON {
		if p.next(); p.cTok != token.IDENT {
			p.errorHandler(p.curPos(), "expect for loop label but got %s", p.cTok)
			return
		}
		label = p.cLit
		p.next()
	}
	ioffs, ilit := p.cOffs, p.cLit
	var (
		i, value *ast.Ident
		oprd     ast.Node
		lrange   *ast.Interval
		blcOffs  int
	)
	if p.cTok == token.IDENT {
		p.next()
		if p.cTok == token.COMMA {
			// for in array, map or string
			p.next()
			voffs, vlit := p.cOffs, p.cLit
			if p.expect(token.IDENT) == -1 {
				return
			}
			if p.expect(token.IN) == -1 {
				return
			}
			var tok token.Token
			oprd, tok = p.parseOperand()
			switch tok {
			case token.INTEGER, token.FLOAT, token.BOOLEAN:
				p.errorHandler(oprd.Position(), "for loop can iterate through %s", tok)
				return
			}
			blcOffs = p.cOffs
			if p.expect(token.LBRACE) == -1 {
				return
			}
			i = &ast.Ident{Base: &ast.Base{Offset: ioffs, File: p.tfile}, Name: ilit}
			value = &ast.Ident{Base: &ast.Base{Offset: voffs, File: p.tfile}, Name: vlit}
		} else {
			// a range loop
			if p.expect(token.IN) == -1 {
				return
			}
			lrange = p.parseInterval()
			blcOffs = p.cOffs
			if p.expect(token.LBRACE) == -1 {
				return
			}
			i = &ast.Ident{Base: &ast.Base{Offset: ioffs, File: p.tfile}, Name: ilit}
		}
	} else if p.expect(token.LBRACE) == -1 {
		return
	}
	bstmt := &ast.BlockStatement{Base: &ast.Base{Offset: blcOffs, File: p.tfile}}
	if p.parseBlock(true, bstmt) {
		p.block.Append(&ast.ForStatement{
			Base:  &ast.Base{Offset: offs, File: p.tfile},
			Label: label,
			I:     i,
			Value: value,
			Oprnd: oprd,
			Range: lrange,
			Insts: bstmt,
		})
	}
}

func (p *parser) parseIf(inForLoop bool, elstmt *ast.ElseStatement) {
	offs := p.cOffs

	// switch p.nTok {
	// case token.ON:
	// 	p.next()
	// 	condOffs := p.cOffs
	// 	switch p.next(); p.cTok {
	// 	case token.LINUX, token.MACOS, token.WINDOWS:
	// 		cond = &ast.OSysCheck{Base: &ast.Base{Offset: condOffs, File: p.tfile}, OS: p.cTok}
	// 		p.next()
	// 	default:
	// 		p.errorHandler(p.curPos(), "expect operating system keyword got %s (%s)", p.cTok, p.cLit)
	// 		return
	// 	}
	// case token.HASH, token.AT, token.FD:
	// 	p.next()
	// 	op := p.cTok
	// 	base := &ast.Base{Offset: p.cOffs, File: p.tfile}
	// 	var x ast.Node
	// 	if p.next(); p.cTok == token.IDENT || (p.cTok == token.STRING && op == token.FD) {
	// 		soffs, tok, lit := p.cOffs, p.cTok, p.cLit
	// 		if p.next(); p.cTok != token.EXISTS {
	// 			p.errorHandler(p.curPos(), "expect %s but got %s", token.EXISTS, p.cTok)
	// 			return
	// 		}
	// 		if tok == token.STRING {
	// 			x = &ast.BasicLit{Base: base, Lit: lit, Kind: tok, Mark: p.s.src[soffs-1]}
	// 		} else {
	// 			x = &ast.Ident{Base: base, Name: lit}
	// 		}
	// 	} else {
	// 		p.errorHandler(p.curPos(), "expect identifier or file expression but got %s", p.cTok)
	// 		return
	// 	}
	// 	cond = &ast.Exists{Base: base, Op: op, X: x}
	// 	p.next()
	// default:

	cond := p.parseBinaryExpr(false, token.LowestPrec+1)
	blcOffs := p.cOffs
	if p.expect(token.LBRACE) == -1 {
		return
	}
	bstmt := &ast.BlockStatement{Base: &ast.Base{Offset: blcOffs, File: p.tfile}}
	if p.parseBlock(inForLoop, bstmt) {
		ifstmt := &ast.IfStatement{
			Base:  &ast.Base{Offset: offs, File: p.tfile},
			Cond:  cond,
			Insts: bstmt,
		}
		if elstmt == nil {
			p.block.Append(ifstmt)
		} else {
			elstmt.IfStmt = ifstmt
		}
		// parse else statement if there any
		if p.cTok != token.ELSE {
			return
		}
		elOffs := p.cOffs
		p.next()
		elstmt := &ast.ElseStatement{
			Base: &ast.Base{Offset: elOffs, File: p.tfile},
		}
		if p.cTok == token.IF {
			p.parseIf(inForLoop, elstmt)
		} else {
			blcOffs = p.cOffs
			if p.expect(token.LBRACE) == -1 {
				return
			}
			elstmt.Insts = &ast.BlockStatement{Base: &ast.Base{Offset: blcOffs, File: p.tfile}}
			if !p.parseBlock(inForLoop, elstmt.Insts) {
				return
			}
		}
		ifstmt.Else = elstmt
	}
}

func (p *parser) parseBlock(inForLoop bool, block *ast.BlockStatement) bool {
	prevBlock := p.block
	p.block = block
	defer func() { p.block = prevBlock }()
	for p.cTok != token.RBRACE && p.cTok != token.EOF {
		switch p.cTok {
		case token.IDENT:
			if p.nTok == token.INC || p.nTok == token.DEC {
				offs, lit := p.cOffs, p.cLit
				p.next()
				p.block.Append(&ast.ExprWrapperStatement{
					X: &ast.IncDec{
						Op: p.cTok,
						X:  &ast.Ident{Base: &ast.Base{Offset: offs, File: p.tfile}, Name: lit},
					},
				})
				p.next()
				p.expect(token.LF)
			} else {
				if p.nTok == token.LBRACK {
					if x := p.parseIndexExpression(); x != nil {
						p.parseAssignStatement(x)
					}
				} else {
					settable := &ast.Ident{Base: &ast.Base{Offset: p.cOffs, File: p.tfile}, Name: p.cLit}
					p.parseAssignStatement(settable)
				}
			}
		case token.AT, token.HASH:
			// parse invocation command call
			p.parseCallReference(false, nil)
		case token.FOR:
			p.parseForLoop(inForLoop)
		case token.IF:
			p.parseIf(inForLoop, nil)
		case token.EXIT:
			// parse exit
			offs := p.cOffs
			if code := p.parseBinaryExpr(false, token.LowestPrec+1); code != nil {
				p.block.Append(&ast.ExprWrapperStatement{
					X: &ast.Exit{Base: &ast.Base{Offset: offs, File: p.tfile}, ExitCode: code},
				})
			}
			p.expect(token.LF)
		case token.RETURN:
			// parse return
			p.block.Append(&ast.ReturnStatement{
				Base: &ast.Base{Offset: p.cOffs, File: p.tfile},
				X:    p.parseBinaryExpr(false, token.LowestPrec+1),
			})
		case token.BREAK, token.CONTINUE:
			offs, label, op := p.cOffs, "", p.cTok
			p.next()
			if p.cTok == token.COLON {
				p.next()
				label = p.cLit
				if p.expect(token.IDENT) == -1 {
					return false
				}
			}
			p.block.Append(&ast.BreakContinueStatement{
				Base:  &ast.Base{Offset: offs, File: p.tfile},
				Op:    op,
				Label: label,
			})
			if p.cTok == token.LF {
				// skip optional line feed
				p.next()
			}
		case token.COMMENT:
			p.next()
			// eat comment for now
			// TODO: add comment to token file
		}
	}
	endBlock := p.cTok == token.RBRACE
	p.next()
	if p.cTok == token.LF {
		p.next()
	}
	return endBlock
}

func (p *parser) parseIndexExpression() ast.SettableNode {
	offs, lit := p.cOffs, p.cLit
	if p.next(); p.cTok != token.LBRACK {
		p.errorHandler(p.curPos(), "expect [ but got %s", p.cTok)
		return nil
	}
	index := p.parseBinaryExpr(false, token.LowestPrec+1)
	if p.cTok != token.RBRACK {
		return nil
	}
	base := &ast.Base{Offset: offs, File: p.tfile}
	return &ast.Index{
		Base:  base,
		Index: index,
		X:     &ast.Ident{Base: base, Name: lit},
	}
}

func (p *parser) parseInterval() *ast.Interval {
	offs := p.cOffs
	var aic, bic bool
	switch p.cTok {
	case token.LBRACE, token.LPAREN, token.RBRACK:
		p.next()
		aic = false
	case token.LBRACK:
		p.next()
		fallthrough
	default:
		aic = true
	}
	a, _ := p.parseOperand()
	if p.expect(token.RANGE) == -1 {
		return nil
	}
	b, _ := p.parseOperand()
	switch p.cTok {
	case token.RBRACE, token.RPAREN, token.LBRACK:
		p.next()
		bic = false
	case token.RBRACK:
		p.next()
		fallthrough
	default:
		bic = true
	}
	return &ast.Interval{Base: &ast.Base{Offset: offs, File: p.tfile}, A: a, AInclude: aic, B: b, BInclude: bic}
}

func (p *parser) parseTernaryExpr(x ast.Node) ast.Node {
	offs := p.cOffs
	tx := p.parseBinaryExpr(false, token.LowestPrec+1)
	if p.cTok == token.COLON {
		return &ast.Conditional{
			Base:  &ast.Base{Offset: offs, File: p.tfile},
			Cond:  x,
			True:  tx,
			False: p.parseBinaryExpr(false, token.LowestPrec+1),
		}
	} else {
		p.errorHandler(p.curPos(), "expect ':' but got %s", p.cTok)
	}
	return nil
}

func (p *parser) parseFallbackExpr(x ast.Node) ast.Node {
	offs := p.cOffs
	return &ast.Fallback{
		Base:    &ast.Base{Offset: offs, File: p.tfile},
		Primary: x,
		Default: p.parseBinaryExpr(false, token.LowestPrec+1),
	}
}

func (p *parser) parseIsExpr(x ast.Node) ast.Node {
	offs := p.cOffs
	var types []token.Token
	p.next()
	for {
		switch {
		case token.TINTEGER <= p.cTok && p.cTok <= token.TMAP:
			types = append(types, p.cTok)
		case p.cTok == token.OR:
			// do nothing
		default:
			return &ast.IsType{
				Base:  &ast.Base{Offset: offs, File: p.tfile},
				X:     x,
				Types: types,
			}
		}
		p.next()
	}
}

func (p *parser) parseTypeCaseExpr() ast.Node {
	offs := p.cOffs
	to := p.cTok
	p.next()
	if p.cTok == token.LPAREN {
		x := p.parseBinaryExpr(false, token.LowestPrec+1)
		if p.expect(token.RPAREN) != -1 {
			return &ast.TypeCast{
				Base: &ast.Base{Offset: offs, File: p.tfile},
				To:   to,
				X:    x,
			}
		}
	} else {
		p.errorHandler(p.curPos(), "expected (")
	}
	return nil
}

func (p *parser) parseTransformation() ast.Node {
	ioffs, ilit := p.cOffs, p.cLit
	p.next()
	ftOffs := p.cOffs
	if p.expect(token.LPAREN) == -1 {
		return nil
	}
	if fn := p.parseDeclareFunction(true); fn != nil {
		return &ast.Transformation{
			Base:  &ast.Base{Offset: ftOffs, File: p.tfile},
			Ident: &ast.Ident{Base: &ast.Base{Offset: ioffs, File: p.tfile}, Name: ilit},
			Fn:    fn,
		}
	}
	return nil
}

func (p *parser) parseCallReference(assign bool, prev *ast.Call) ast.Node {
	callOffs := p.cOffs
	kind := p.cTok
	p.next()
	name := p.cLit
	if p.expect(token.IDENT) == -1 {
		return nil
	}
	var args []ast.Node
	var redirect *ast.RedirectTo

	for p.cTok != token.LF && p.cTok != token.EOF {
		switch p.cTok {
		case token.WRITE_TO, token.APPEND_TO:
			if redirect != nil {
				p.errorHandler(p.curPos(), "multiple write (>) or append (>>) to")
				return nil
			}
			redirect = &ast.RedirectTo{
				Base:   &ast.Base{Offset: p.cOffs, File: p.tfile},
				Append: p.cTok == token.APPEND_TO,
			}
			p.next()
		case token.READ_FROM:
			if redirect != nil {
				p.errorHandler(p.curPos(), "read from syntax is not allow after write or append to file")
				return nil
			}
			offs := p.cOffs
			p.next()
			x, _ := p.parseOperand()
			args = append(args, &ast.ReadFrom{
				Base: &ast.Base{Offset: offs, File: p.tfile},
				File: x,
			})
		case token.PIPE:
			goto end
		default:
			x, _ := p.parseOperand()
			if redirect != nil {
				redirect.Files = append(redirect.Files, x)
			} else {
				args = append(args, x)
			}
		}
	}
end:
	if tok := p.cTok; p.cTok == token.LF || p.cTok == token.PIPE {
		var node ast.Node
		node = &ast.Call{
			Base: &ast.Base{Offset: callOffs, File: p.tfile},
			Kind: kind,
			Name: name,
			Args: args,
		}

		p.next()
		if tok == token.PIPE {
			nextNode := p.parseCallReference(true, node.(*ast.Call))
			node = &ast.Pipe{
				X: node.(*ast.Call),
				Y: nextNode,
			}
		} else if redirect != nil {
			redirect.Caller = node
			node = redirect
		}

		if assign {
			return node
		} else {
			p.block.Append(&ast.ExprWrapperStatement{X: node})
			return nil
		}
	}
	return nil
}

func (p *parser) parseDeclareFunction(literal bool) *ast.Function {
	var name string
	if !literal {
		name = p.cLit
		if p.expect(token.IDENT) == -1 || p.expect(token.LPAREN) == -1 {
			return nil
		}
	}
	if args := p.parseDeclareArgument(); args == nil {
		return nil
	} else if p.expect(token.RPAREN) != -1 {
		blcOff := p.cOffs
		switch p.cTok {
		case token.LAMBDA:
			if x := p.parseBinaryExpr(false, token.LowestPrec+1); x != nil {
				return &ast.Function{
					Lambda: token.LAMBDA,
					Args:   args,
					X:      x,
				}
			}
		case token.LBRACE:
			p.next()
			block := &ast.BlockStatement{Base: &ast.Base{Offset: blcOff, File: p.tfile}}
			if p.parseBlock(false, block) {
				return &ast.Function{
					Name:  name,
					Args:  args,
					Insts: block,
				}
			}
		default:
			p.errorHandler(p.curPos(), "unexpected token %s", p.cTok)
		}
	}
	return nil
}

func (p *parser) parseDeclareArgument() []*ast.Ident {
	var args []*ast.Ident
	for p.cTok != token.RPAREN {
		if p.cTok == token.IDENT {
			args = append(args, &ast.Ident{Base: &ast.Base{Offset: p.cOffs, File: p.tfile}, Name: p.cLit})
			if p.next(); p.cTok == token.COMMA {
				p.next()
			}
		} else {
			p.errorHandler(p.curPos(), "expect identifier but got %s", p.cTok)
			return nil
		}
	}
	return args
}

func parseArrayFile(n ast.Node, tok token.Token) (isGlob bool, x []ast.Node) {
	if tok == token.STRING {
		bl := n.(*ast.BasicLit)
		if mes, err := filepath.Glob(bl.Lit); err != nil || len(mes) == 0 {
			return false, nil
		} else {
			for _, sf := range mes {
				x = append(x, &ast.BasicLit{Lit: sf, Kind: token.STRING, Mark: bl.Mark})
			}
		}
		return true, x
	} else {
		return false, nil
	}
}
