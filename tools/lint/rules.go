package main

import (
	"go/ast"
	"strings"

	"github.com/mgechev/revive/lint"
)

type rule struct {
	name  string
	apply func(*lint.File, lint.Arguments) []lint.Failure
}

func (r *rule) Name() string {
	return r.name
}

func (r *rule) Apply(file *lint.File, arguments lint.Arguments) []lint.Failure {
	return r.apply(file, arguments)
}

var intIdsRule = &rule{
	name: "int_ids",
	apply: func(file *lint.File, _ lint.Arguments) []lint.Failure {
		var failures []lint.Failure
		ast.Inspect(file.AST, func(node ast.Node) bool {
			n, ok := node.(*ast.StructType)
			if !ok {
				return true
			}
			if n.Fields == nil || n.Fields.List == nil {
				return false
			}
			for _, f := range n.Fields.List {
				tp := f.Type
				if tp == nil || f.Names == nil || len(f.Names) == 0 || !strings.HasSuffix(f.Names[0].Name, "ID") {
					continue
				}
				// if tp is star expression, get the underlying type
				if star, ok := tp.(*ast.StarExpr); ok {
					tp = star.X
				}
				ident, ok := tp.(*ast.Ident)
				if !ok {
					continue
				}
				if ident.Name == "int" {
					failures = append(failures, lint.Failure{
						Failure: "struct fields with a name ending in ID must not be int or *int",
						Node:    f,
					})
				}
			}
			return false
		})
		return failures
	},
}
