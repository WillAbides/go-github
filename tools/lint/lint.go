package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/mgechev/revive/lint"
	"github.com/mgechev/revive/revivelib"
)

type rootCmd struct {
	Format  string   `kong:"enum='ndjson,default',default=default,help='Output format.'"`
	Include []string `kong:"arg,help='Paths to lint.',default='./...'"`
}

func (r *rootCmd) Run(k *kong.Context) error {
	emptyConfig := &lint.Config{Rules: map[string]lint.RuleConfig{}}
	extraRules := []revivelib.ExtraRule{
		{Rule: intIdsRule},
	}
	revive, err := revivelib.New(emptyConfig, true, 0, extraRules...)
	if err != nil {
		return err
	}
	var include []*revivelib.LintPattern
	for i := range r.Include {
		include = append(include, revivelib.Include(r.Include[i]))
	}
	failures, err := revive.Lint(include...)
	if err != nil {
		return err
	}
	format := r.Format
	if format == "default" && os.Getenv("GITHUB_ACTIONS") != "" {
		format = "ndjson"
	}
	output, status, err := revive.Format(format, failures)
	if err != nil {
		return err
	}
	fmt.Fprint(k.Stdout, output)
	if status != 0 {
		k.Exit(status)
	}
	return nil
}

func main() {
	var cli rootCmd
	p := kong.Parse(&cli)
	err := p.Run()
	p.FatalIfErrorf(err)
}
