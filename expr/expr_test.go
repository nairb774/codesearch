package expr

import (
	"testing"

	"github.com/golang/protobuf/proto"
)

func TestParsing(t *testing.T) {
	tests := []struct {
		expr string
		want []*ExpressionPart
	}{
		{
			expr: `abc`,
			want: []*ExpressionPart{{Expression: `abc`}},
		},
		{
			expr: `+abc`,
			want: []*ExpressionPart{{Code: ExpressionPart_MATCH, Expression: `abc`}},
		},
		{
			expr: `-abc`,
			want: []*ExpressionPart{{Negated: true, Code: ExpressionPart_MATCH, Expression: `abc`}},
		},
		{
			expr: `f:abc`,
			want: []*ExpressionPart{{Code: ExpressionPart_FILE, Expression: `abc`}},
		},
		{
			expr: `file:abc`,
			want: []*ExpressionPart{{Code: ExpressionPart_FILE, Expression: `abc`}},
		},
		{
			expr: `-f:abc`,
			want: []*ExpressionPart{{Negated: true, Code: ExpressionPart_FILE, Expression: `abc`}},
		},
		{
			expr: `-"abc"`,
			want: []*ExpressionPart{{Negated: true, Code: ExpressionPart_MATCH, Expression: `"abc"`}},
		},
		{
			expr: `-\"abc"`,
			want: []*ExpressionPart{{Negated: true, Code: ExpressionPart_MATCH, Expression: `"abc"`}},
		},
		{
			expr: `(-f:abc)`,
			want: []*ExpressionPart{{Expression: `-f:abc`}},
		},
		{
			expr: `[]]`,
			want: []*ExpressionPart{{Expression: `\]`}},
		},
		{
			expr: `[][:alpha:]]`,
			want: []*ExpressionPart{{Expression: `[A-Z\]a-z]`}},
		},
		{
			expr: `[][]`,
			want: []*ExpressionPart{{Expression: `[\[\]]`}},
		},
		{
			expr: `[^]]`,
			want: []*ExpressionPart{{Expression: `[^\]]`}},
		},
		{
			expr: `[^][]`,
			want: []*ExpressionPart{{Expression: `[^\[\]]`}},
		},
		{
			expr: `"\("`,
			want: []*ExpressionPart{{Expression: `"\("`}},
		},
		{
			expr: `:`,
			want: []*ExpressionPart{{Expression: `:`}},
		},
		{
			expr: `-:`,
			want: []*ExpressionPart{{Negated: true, Code: ExpressionPart_MATCH, Expression: `:`}},
		},
		{
			expr: `\-:`,
			want: []*ExpressionPart{{Expression: `-:`}},
		},
		{
			expr: `-a(b(c)d(e)f(g(h)i)j)k`,
			want: []*ExpressionPart{{Negated: true, Code: ExpressionPart_MATCH, Expression: `abcdefghijk`}},
		},
	}

	for _, test := range tests {
		want := &Expression{Parts: test.want}
		if got, err := Parse(test.expr); err != nil || !proto.Equal(got, want) {
			t.Errorf("parse(%q)=(%v, %v) want (%v, nil)", test.expr, got, err, want)
		}
	}
}

func TestParsingErrors(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{
			expr: "",
			want: "no expression found",
		},
		{
			expr: " ",
			want: "no expression found",
		},
		{
			expr: " f: ",
			want: "FILE followed by empty expression",
		},
		{
			expr: "(())",
			want: "expression around 0 to 4 is empty",
		},
		{
			expr: " f:(?:) ",
			want: "expression around 3 to 7 is empty",
		},
	}

	for _, test := range tests {
		if got, err := Parse(test.expr); err == nil || err.Error() != test.want {
			t.Errorf("parse(%q)=(%v, %v) want (nil, %v)", test.expr, got, err, test.want)
		}
	}
}
