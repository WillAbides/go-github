module tools

go 1.20

require (
	github.com/alecthomas/kong v0.8.1
	github.com/mgechev/revive v1.3.4
)

require (
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/chavacava/garif v0.1.0 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/fatih/structtag v1.2.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/mgechev/dots v0.0.0-20210922191527-e955255bf517 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/tools v0.14.0 // indirect
)

// until https://github.com/mgechev/revive/pull/921 is merged
replace github.com/mgechev/revive => github.com/willabides/revive v0.0.0-20231025002332-4cc2992b0c2d
