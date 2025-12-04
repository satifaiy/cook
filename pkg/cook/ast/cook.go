package ast

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"

	"github.com/cozees/cook/pkg/cook/token"
	"github.com/cozees/cook/pkg/runtime/args"
)

const (
	TargetInitialize = "initialize"
	TargetFinalize   = "finalize"
	TargetAll        = "all"
)

type Cook interface {
	Code
	Block() *BlockStatement
	AddFunction(fn *Function)
	AddTarget(base *Base, name string) (*Target, error)
	Execute(pargs map[string]any) error
	ExecuteWithTarget(pargs map[string]any, names ...string) error
	Scope() Scope
}

type cook struct {
	ctx *xContext

	targets       map[string]int
	targetIndexes []*Target
	fns           map[string]*Function

	initializeTargets Targets
	finalizeTargets   Targets
	targetAll         *Target
	Insts             *BlockStatement
}

func NewCook() Cook {
	return &cook{
		targets: make(map[string]int),
		fns:     make(map[string]*Function),
		Insts:   &BlockStatement{root: true, plain: true},
	}
}

func (c *cook) Block() *BlockStatement { return c.Insts }
func (c *cook) Scope() Scope           { return c.ctx.scope }

func (c *cook) AddTarget(base *Base, name string) (*Target, error) {
	switch name {
	case TargetInitialize:
		if err := c.initializeTargets.notExisted(base.File.Name()); err != nil {
			return nil, err
		}
		t := &Target{Base: base, name: name, Insts: &BlockStatement{Base: base}}
		c.initializeTargets = append(c.initializeTargets, t)
		return t, nil
	case TargetFinalize:
		if err := c.finalizeTargets.notExisted(base.File.Name()); err != nil {
			return nil, err
		}
		t := &Target{Base: base, name: name, Insts: &BlockStatement{Base: base}}
		c.finalizeTargets = append(c.finalizeTargets, t)
		return t, nil
	case TargetAll:
		if c.targetAll != nil {
			return nil, fmt.Errorf("target has been declare at %s", c.targetAll.ErrPos())
		}
		c.targetAll = &Target{Base: base, name: name, Insts: &BlockStatement{Base: base}}
		return c.targetAll, nil
	}

	if ti, ok := c.targets[name]; !ok {
		t := &Target{Base: base, name: name, Insts: &BlockStatement{Base: base}}
		c.targetIndexes = append(c.targetIndexes, t)
		c.targets[name] = len(c.targetIndexes) - 1
		return t, nil
	} else {
		pos := c.targetIndexes[ti].Position()
		return nil, fmt.Errorf("target %s already exist, previously define at %d:%d", name, pos.Line, pos.Column)
	}

}

func (c *cook) AddFunction(fn *Function) { c.fns[fn.Name] = fn }

func (c *cook) Execute(pargs map[string]any) error {
	if c.targetAll == nil {
		return errors.New("default target all is not defined")
	}
	return c.ExecuteWithTarget(pargs, TargetAll)
}

func (c *cook) ExecuteWithTarget(pargs map[string]any, names ...string) (err error) {
	c.ctx = c.renewContext()
	for name, v := range pargs {
		c.ctx.scope.SetVariable(name, v, reflect.ValueOf(v).Kind(), nil)
	}
	// execute outter statement
	if c.Insts != nil {
		if err = c.Insts.Evaluate(c.ctx); err != nil {
			return err
		}
	}
	// execute initialize
	for _, init := range c.initializeTargets {
		// initialize target can set variable to globally thus no need to create scope for it
		if err := init.Execute(c.ctx, nil); err != nil {
			return err
		}
	}
	// defer for finalize
	defer func() {
		for _, final := range c.finalizeTargets {
			if ferr := final.Execute(c.ctx, nil); ferr != nil {
				// igore the error from finalize display warning instead
				fmt.Fprintf(os.Stderr, "Error while executing finalize target %s: %s\n", final.ErrPos(), ferr)
			}
		}
	}()

	// if all target is need and using syntax "all: *" then we execute every target in the order
	// of its declaration
	if len(names) == 1 && names[0] == TargetAll {
		c.ctx.EnterBlock(false, "")
		if c.targetAll == nil {
			return errors.New("target all is not defined in any Cookfile")
		}
		if c.targetAll.all {
			for _, t := range c.targetIndexes {
				c.ctx.EnterBlock(false, "")
				if err = t.Execute(c.ctx, nil); err != nil {
					return err
				}
				c.ctx.ExitBlock(-1)
			}
		} else if err = c.targetAll.Execute(c.ctx, nil); err != nil {
			return err
		}
		c.ctx.ExitBlock(-1)
	} else {
		// each target must execute with it's own scope
		for _, name := range names {
			if name == TargetAll {
				fmt.Println("warning: target all was include among other, it won't be executed.")
				continue
			}
			c.ctx.EnterBlock(false, "")
			if err = c.ctx.GetTarget(name).Execute(c.ctx, nil); err != nil {
				return err
			}
			c.ctx.ExitBlock(-1)
		}
	}
	return nil
}

func (c *cook) renewContext() *xContext {
	return &xContext{
		scope:      &xScope{vars: make(map[string]*ivar)},
		cook:       c,
		continueAt: -1,
		breakAt:    -1,
	}
}

type Targets []*Target

func (t Targets) Len() int           { return len(t) }
func (t Targets) Less(i, j int) bool { return t[i].File.Name() < t[j].File.Name() }
func (t Targets) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t Targets) notExisted(fname string) error {
	if t != nil {
		i := sort.Search(len(t), func(i int) bool {
			return t[i].File.Name() >= fname
		})
		if i < len(t) && t[i].File.Name() == fname {
			pos := t[i].Position()
			return fmt.Errorf("defined %s target, previously exit at %d:%d", TargetInitialize, pos.Line, pos.Column)
		}
	}
	return nil
}

type Target struct {
	*Base
	all   bool
	Insts *BlockStatement
	name  string
}

func (t *Target) SetCallAll() {
	if t.name != TargetAll {
		panic("cook internal error: set call all on a none all target")
	}
	t.all = true
}

func (t *Target) Execute(ctx Context, args []*args.FunctionArg) error {
	scope, _ := ctx.EnterBlock(false, "")
	defer ctx.ExitBlock(-1)
	for i, fa := range args {
		scope.SetVariable(strconv.Itoa(i+1), fa.Val, fa.Kind, nil)
	}
	scope.SetVariable("0", int64(len(args)), reflect.Int64, nil)
	return t.Insts.Evaluate(ctx)
}

func (t *Target) Vist(cb CodeBuilder) {
	cb.WriteString(t.name)
	cb.WriteString(":\n")
	t.Insts.Visit(cb)
}

// a callback function to provide caller to set value for each argument of the function
type argumentSetter func(int) (any, reflect.Kind, error)

type Function struct {
	Insts  *BlockStatement
	Name   string
	Lambda token.Token
	X      Node
	Args   []*Ident
}

func (fn *Function) Execute(ctx Context, pargs []Node) (v any, kind reflect.Kind, err error) {
	return fn.internalExecute(ctx, len(pargs), func(i int) (any, reflect.Kind, error) {
		return pargs[i].Evaluate(ctx)
	})
}

func (fn *Function) internalExecute(ctx Context, numArgs int, farg argumentSetter) (v any, kind reflect.Kind, err error) {
	switch {
	case numArgs > len(fn.Args):
		return nil, 0, fmt.Errorf("too many argument defined %d, given %d", len(fn.Args), numArgs)
	case numArgs < len(fn.Args):
		return nil, 0, fmt.Errorf("not enought argument defined %d, given %d", len(fn.Args), numArgs)
	}

	scope, _ := ctx.EnterBlock(false, "")
	defer ctx.ExitBlock(-1)
	for i := 0; i < numArgs; i++ {
		if v, k, err := farg(i); err != nil {
			return nil, 0, err
		} else {
			scope.SetVariable(fn.Args[i].Name, v, k, nil)
		}
	}
	if fn.Lambda == token.LAMBDA {
		return fn.X.Evaluate(ctx)
	} else if err = fn.Insts.Evaluate(ctx); err == nil {
		v, kind = ctx.GetReturnValue()
	}
	return v, kind, err
}
