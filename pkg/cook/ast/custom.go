package ast

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/cozees/cook/pkg/cook/token"
)

const (
	TransformSlice reflect.Kind = reflect.UnsafePointer + 1000 + iota
	TransformMap
)

type iTransform struct {
	Len    func() int
	Source func(ctx Context, i any) (any, reflect.Kind, error)
	Value  func(ctx Context, i, val any) (any, reflect.Kind, error)
}

func (it *iTransform) Transform(ctx Context, index any) (any, reflect.Kind, error) {
	if v, _, err := it.Source(ctx, index); err != nil {
		return nil, 0, err
	} else {
		return it.Value(ctx, index, v)
	}
}

type StringInterpolation struct {
	*Base
	mark  byte
	raw   string
	pos   []int
	nodes []Node
}

func (si *StringInterpolation) Evaluate(ctx Context) (any, reflect.Kind, error) {
	if len(si.nodes) == 0 || len(si.pos) == 0 || len(si.nodes) != len(si.pos) {
		panic("cook internal error: string interpolation not properly constructed")
	}
	builder, offs := &strings.Builder{}, 0
	for i := 0; i < len(si.pos); i++ {
		if offs < si.pos[i] {
			builder.WriteString(si.raw[offs:si.pos[i]])
			offs = si.pos[i]
		}
		if v, k, err := si.nodes[i].Evaluate(ctx); err != nil {
			return nil, 0, err
		} else if str, err := convertToString(ctx, v, k); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", si.ErrPos(), err)
		} else {
			builder.WriteString(str)
		}
	}
	if offs < len(si.raw) {
		builder.WriteString(si.raw[offs:])
	}
	return builder.String(), reflect.String, nil
}

type StringInterpolationBuilder interface {
	io.StringWriter
	AddExpression(node Node)
	Build(offset int, file *token.File) Node
}

type stringInterpolationBuilder struct {
	*strings.Builder
	mark  byte
	pos   []int
	nodes []Node
}

func NewStringInterpolationBuilder(mark byte) StringInterpolationBuilder {
	return &stringInterpolationBuilder{Builder: &strings.Builder{}, mark: mark}
}

func (sib *stringInterpolationBuilder) AddExpression(node Node) {
	sib.pos = append(sib.pos, sib.Len())
	sib.nodes = append(sib.nodes, node)
}

func (sib *stringInterpolationBuilder) Build(offset int, file *token.File) Node {
	return &StringInterpolation{Base: &Base{Offset: offset, File: file}, pos: sib.pos, nodes: sib.nodes, mark: sib.mark, raw: sib.String()}
}
