package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mgechev/revive/lint"
	"github.com/mgechev/revive/revivelib"
)

const usage = `Usage: lint <path>

Runs go-github's custom lint rules on the given path.
`

func main() {
	var include []*revivelib.LintPattern
	args := os.Args[1:]
	showUsage(args)
	for i := range args {
		include = append(include, revivelib.Include(args[i]))
	}
	emptyConfig := &lint.Config{Rules: map[string]lint.RuleConfig{}}
	extraRules := []revivelib.ExtraRule{
		{Rule: intIdsRule},
	}
	revive, err := revivelib.New(emptyConfig, true, 0, extraRules...)
	exitOnErr(err)
	failures, err := revive.Lint(include...)
	exitOnErr(err)
	output, exitCode, err := revive.Format("default", failures)
	exitOnErr(err)
	fmt.Print(output)
	os.Exit(exitCode)
}

func showUsage(args []string) {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
}

func exitOnErr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "error running lint: %v\n", err)
	os.Exit(1)
}
