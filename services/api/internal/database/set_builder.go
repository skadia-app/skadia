package database

import (
	"fmt"
	"strings"
)

type setBuilder struct {
	sets []string
	args []any
	idx  int
}

func newSetBuilder(startIdx int) *setBuilder {
	return &setBuilder{idx: startIdx}
}

func (b *setBuilder) Set(col string, val any) {
	b.sets = append(b.sets, fmt.Sprintf("%s = $%d", col, b.idx))
	b.args = append(b.args, val)
	b.idx++
}

func (b *setBuilder) SetExpr(expr string, vals ...any) {
	b.sets = append(b.sets, expr) // for complex expressions or literals
	for _, v := range vals {
		b.args = append(b.args, v)
	}
	// caller manages $N placeholders in expr
}

func (b *setBuilder) HasSets() bool   { return len(b.sets) > 0 }
func (b *setBuilder) Clause() string  { return strings.Join(b.sets, ", ") }
func (b *setBuilder) Args() []any     { return b.args }
func (b *setBuilder) NextIdx() int    { return b.idx }
