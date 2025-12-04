package ast

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/cozees/cook/pkg/runtime/function"
)

type Scope interface {
	GetVariable(name string) (value any, kind reflect.Kind, fromEnv bool)
	SetVariable(name string, value any, kind reflect.Kind, bubble func(v any, k reflect.Kind) error) bool
	SetReturnValue(v any, kind reflect.Kind)
	GetReturnValue() (v any, kind reflect.Kind)
}

type ivar struct {
	value  any
	kind   reflect.Kind
	bubble func(v any, kind reflect.Kind) error
}

type xScope struct {
	parent       *xScope
	hasChild     bool
	returnResult *ivar
	vars         map[string]*ivar
}

func (xs *xScope) GetVariable(name string) (value any, kind reflect.Kind, fromEnv bool) {
	if iv, ok := xs.vars[name]; !ok {
		if xs.parent == nil {
			goto tryEnv
		}
		if value, kind, fromEnv = xs.parent.GetVariable(name); kind != reflect.Invalid {
			return
		}
	} else {
		value, kind = iv.value, iv.kind
		return
	}
tryEnv:
	if value = os.Getenv(name); value.(string) != "" {
		kind, fromEnv = reflect.String, true
	}
	return nil, 0, false
}

func (xs *xScope) SetVariable(name string, value any, kind reflect.Kind, bubble func(v any, k reflect.Kind) error) bool {
	switch kind {
	case reflect.Int64, reflect.Float64, reflect.Bool, reflect.String, reflect.Slice, reflect.Map, TransformSlice, TransformMap:
	default:
		panic(fmt.Sprintf("cook internal error: variable '%s' value: %v has an invalid type %s", name, value, kind))
	}

	if iv, ok := xs.vars[name]; ok {
		iv.value, iv.kind = value, kind
		if iv.bubble != nil {
			iv.bubble(value, kind)
		}
	} else if xs.hasChild {
		return xs.parent != nil && xs.parent.SetVariable(name, value, kind, bubble)
	} else if xs.parent == nil || !xs.parent.SetVariable(name, value, kind, bubble) {
		// we here mean not variable is no exist anywhere
		xs.vars[name] = &ivar{value: value, kind: kind, bubble: bubble}
	}
	return true
}

func (xs *xScope) SetReturnValue(v any, kind reflect.Kind) {
	xs.returnResult = &ivar{value: v, kind: kind}
}

func (xs *xScope) GetReturnValue() (v any, kind reflect.Kind) {
	if xs.returnResult != nil {
		v, kind = xs.returnResult.value, xs.returnResult.kind
		xs.returnResult = nil
	}
	return
}

type Context interface {
	Scope
	EnterBlock(forLoop bool, loopLabel string) (Scope, int)
	ExitBlock(index int)
	ShouldBreak(fromLoop bool) bool
	ResetBreakContinue()
	Break(label string) error
	Continue(label string) error
	GetCommand(name string) function.Function
	GetTarget(name string) *Target
	GetFunction(name string) *Function
}

type xContext struct {
	scope *xScope
	cook  *cook
	// for loop properties for break & continue
	loopsLabel []string
	continueAt int
	breakAt    int
	loops      []int
}

func (xc *xContext) GetVariable(name string) (value any, kind reflect.Kind, fromEnv bool) {
	return xc.scope.GetVariable(name)
}

func (xc *xContext) SetVariable(name string, value any, kind reflect.Kind, bubble func(v any, k reflect.Kind) error) bool {
	return xc.scope.SetVariable(name, value, kind, bubble)
}

func (xc *xContext) SetReturnValue(v any, kind reflect.Kind) {
	xc.scope.SetReturnValue(v, kind)
}

func (xc *xContext) GetReturnValue() (v any, kind reflect.Kind) {
	return xc.scope.GetReturnValue()
}

func (xc *xContext) GetFunction(name string) *Function        { return xc.cook.fns[name] }
func (xc *xContext) GetCommand(name string) function.Function { return function.GetFunction(name) }
func (xc *xContext) GetTarget(name string) *Target {
	if ind, ok := xc.cook.targets[name]; ok && len(xc.cook.targetIndexes) > 0 {
		return xc.cook.targetIndexes[ind]
	} else {
		return nil
	}
}

func (xc *xContext) EnterBlock(forLoop bool, loopLabel string) (Scope, int) {
	xc.scope = &xScope{parent: xc.scope, vars: make(map[string]*ivar)}
	loopIndex := -1
	if forLoop {
		xc.loopsLabel = append(xc.loopsLabel, loopLabel)
		loopIndex = len(xc.loopsLabel) - 1
		xc.loops = append(xc.loops, loopIndex)
	}
	return xc.scope, loopIndex
}

func (xc *xContext) ExitBlock(index int) {
	if index >= 0 {
		if index != len(xc.loopsLabel)-1 {
			panic(fmt.Sprintf("wrong exit loop block index %d in a loop %d", index, len(xc.loopsLabel)))
		}
		xc.loopsLabel = xc.loopsLabel[:index]
		xc.loops = xc.loops[:len(xc.loops)-1]
	}
	if xc.scope.parent == nil {
		panic("exit block call on outter block")
	}
	xc.scope = xc.scope.parent
}

func (xc *xContext) ShouldBreak(fromLoop bool) bool {
	if len(xc.loopsLabel) > 0 {
		loop := xc.currentLoop()
		return (xc.breakAt >= 0 && xc.breakAt <= loop) ||
			(xc.continueAt >= 0 && (xc.continueAt < loop || (!fromLoop && xc.continueAt == loop)))
	}
	return false
}

func (xc *xContext) ResetBreakContinue() { xc.breakAt, xc.continueAt = -1, -1 }

func (xc *xContext) ShouldContinue() bool {
	loop := xc.currentLoop()
	return len(xc.loopsLabel) > 0 && xc.breakAt >= 0 && xc.continueAt >= 0 && xc.continueAt == loop
}

func (xc *xContext) currentLoop() int {
	loop := -1
	if len(xc.loops) > 0 {
		loop = xc.loops[len(xc.loops)-1]
	}
	return loop
}

func (xc *xContext) Break(label string) error {
	if len(xc.loopsLabel) == 0 {
		return errors.New("statement break can only be use inside for loop")
	}
	// break on current loop, no need to look for outter loop
	if label == "" {
		xc.breakAt = len(xc.loopsLabel) - 1
	} else {
		for i := len(xc.loopsLabel) - 1; i >= 0; i-- {
			if xc.loopsLabel[i] == label {
				xc.breakAt = i
				return nil
			}
		}
	}
	return fmt.Errorf("break at lable %s not exist", label)
}

func (xc *xContext) Continue(label string) error {
	if len(xc.loopsLabel) == 0 {
		return errors.New("statement continue can only be use inside for loop")
	}
	if label == "" {
		xc.continueAt = len(xc.loopsLabel) - 1
	} else {
		for i := len(xc.loopsLabel) - 1; i >= 0; i-- {
			if xc.loopsLabel[i] == label {
				xc.continueAt = i
				return nil
			}
		}
	}
	return fmt.Errorf("continue at lable %s not exist", label)
}
