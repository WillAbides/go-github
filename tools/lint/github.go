package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/mgechev/revive/lint"
)

var githubActionsFormatter = &formatter{
	name: "github-actions",
	format: func(failures <-chan lint.Failure, _ lint.Config) (string, error) {
		var buf bytes.Buffer
		for f := range failures {
			var opts []string
			if f.Position.Start.IsValid() {
				opts = append(opts, fmt.Sprintf("file=%s", f.Position.Start.Filename))
				opts = append(opts, fmt.Sprintf("line=%d", f.Position.Start.Line))
				if f.Position.Start.Column != 0 {
					opts = append(opts, fmt.Sprintf("col=%d", f.Position.Start.Column))
				}
			}
			if f.Position.End.IsValid() {
				opts = append(opts, fmt.Sprintf("endLine=%d", f.Position.End.Line))
				if f.Position.End.Column != 0 {
					opts = append(opts, fmt.Sprintf("endCol=%d", f.Position.End.Column))
				}
			}
			optsString := strings.Join(opts, ",")
			line := fmt.Sprintf("::%s %s::%s", "error", optsString, f.Failure)
			buf.WriteString(strings.ReplaceAll(line, "\n", "%0A"))
			buf.WriteRune('\n')
		}
		return buf.String(), nil
	},
}

type formatter struct {
	name   string
	format func(<-chan lint.Failure, lint.Config) (string, error)
}

func (f *formatter) Format(failures <-chan lint.Failure, config lint.Config) (string, error) {
	return f.format(failures, config)
}

func (f *formatter) Name() string {
	return f.name
}
