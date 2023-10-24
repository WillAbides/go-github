package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/mgechev/revive/config"
	"github.com/mgechev/revive/lint"
	"github.com/mgechev/revive/revivelib"
)

type rootCmd struct {
	Format  string   `kong:"enum='github-actions,default',default=default,help='Output format.'"`
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
	exitStatus := 0
	if len(failures) > 0 {
		exitStatus = 1
	}
	fmter, err := config.GetFormatter("default")
	if err != nil {
		return err
	}
	if r.Format == "github-actions" || os.Getenv("GITHUB_ACTIONS") != "" {
		fmter = githubActionsFormatter
	}
	output, err := fmter.Format(failures, *emptyConfig)
	if err != nil {
		return err
	}
	fmt.Fprint(k.Stdout, output)
	if exitStatus != 0 {
		k.Exit(exitStatus)
	}
	return nil
}

func main() {
	var cli rootCmd
	p := kong.Parse(&cli)
	err := p.Run()
	p.FatalIfErrorf(err)
}
