package ast

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/cozees/cook/pkg/cook/token"
)

type Statement interface {
	Code
	Evaluate(ctx Context) error
}

// BlockStatement implement Node contain multiple Statement
type BlockStatement struct {
	*Base
	root  bool // if BlockStatement were use for Cook initial statement
	plain bool // for Cook or Target we don't print {}
	Stmts []Statement
}

func (bs *BlockStatement) Append(stmt Statement) { bs.Stmts = append(bs.Stmts, stmt) }

func (bs *BlockStatement) Evaluate(ctx Context) (err error) {
	for _, stmt := range bs.Stmts {
		if err = stmt.Evaluate(ctx); err != nil {
			return err
		} else if ctx.ShouldBreak(false) {
			break
		}
	}
	return nil
}

// AssignStatement implement Node and handle all assign operation
type AssignStatement struct {
	*Base
	Ident SettableNode
	Op    token.Token
	Value Node
}

func (as *AssignStatement) Evaluate(ctx Context) (err error) {
	if tf, ok := as.Value.(*Transformation); ok {
		tf.to = as.Ident
		_, _, err = tf.Evaluate(ctx)
		return
	}

	var i any
	var k reflect.Kind
	var mMerge *MergeMap
	if ce, ok := as.Value.(*Call); ok {
		ce.OutputResult = true
	} else if mMerge, ok = as.Value.(*MergeMap); ok {
		if as.Op != token.ADD_ASSIGN {
			return errors.New("invalid operator use with merge map syntax, only += operator is allowed")
		}
		goto avoidEvaluate
	}

	// don't evaluate map merge
	i, k, err = as.Value.Evaluate(ctx)
	if err != nil {
		return err
	}

avoidEvaluate:
	switch {
	case as.Op == token.ASSIGN:
		var bubble func(v any, k reflect.Kind) error
		_, isVar := as.Ident.(*Ident)
		ix, isIndex := as.Value.(*Index)
		// we bubble to update original data
		if isVar && isIndex {
			bubble = func(v any, k reflect.Kind) error {
				return ix.Set(ctx, v, k, nil)
			}
		}
		return as.Ident.Set(ctx, i, k, bubble)
	default:
		v, vk, err := as.Ident.Evaluate(ctx)

		// if is a merge map statement
		if mMerge != nil {
			err = mMerge.Set(ctx, v, vk)
			return err
		}

		// regular assign statement
		if err != nil {
			return err
		} else if v == nil {
			return fmt.Errorf("variable %s does not exist", as.Ident.VariableName())
		} else {
			switch as.Op {
			case token.ADD_ASSIGN:
				if sum, sk, err := addOperator(ctx, v, i, vk, k); err != nil {
					return err
				} else {
					as.Ident.Set(ctx, sum, sk, nil)
					return nil
				}
			case token.SUB_ASSIGN, token.MUL_ASSIGN, token.QUO_ASSIGN, token.REM_ASSIGN:
				if r, rk, err := numOperator(ctx, as.Op-5, v, i, vk, k); err != nil {
					return err
				} else {
					as.Ident.Set(ctx, r, rk, nil)
					return nil
				}
			default:
				panic("illegal state parser. Parser should verify the permitted operator already")
			}
		}
	}
}

type BreakContinueStatement struct {
	*Base
	Label string
	Op    token.Token // break or continue
}

func (bcs *BreakContinueStatement) Evaluate(ctx Context) error {
	switch bcs.Op {
	case token.BREAK:
		ctx.Break(bcs.Label)
	case token.CONTINUE:
		ctx.Continue(bcs.Label)
	default:
		panic(fmt.Sprintf("invalid token %s", bcs.Op))
	}
	return nil
}

type ExprWrapperStatement struct {
	X Node
}

func (ews *ExprWrapperStatement) Position() token.Position { return ews.X.Position() }
func (ews *ExprWrapperStatement) ErrPos() string           { return ews.X.ErrPos() }

// IncDecExpr Evaluate increase or delete value by 1
func (ews *ExprWrapperStatement) Evaluate(ctx Context) (err error) {
	_, _, err = ews.X.Evaluate(ctx)
	return
}

// A statement represent return expression return
type ReturnStatement struct {
	*Base
	X Node
}

// Return Evaluate return/forward value of a literal value or a value of a variable
func (r *ReturnStatement) Evaluate(ctx Context) error {
	if v, k, err := r.X.Evaluate(ctx); err == nil {
		ctx.SetReturnValue(v, k)
		return nil
	} else {
		return err
	}
}
