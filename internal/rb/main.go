package rb

import (
	"fmt"
	"os"
	"sort"
)

type command struct {
	run  func(args []string) int
	help string
}

var commands = map[string]command{}

func register(name, help string, run func(args []string) int) {
	commands[name] = command{run: run, help: help}
}

func Main(args []string) int {
	if len(args) == 0 {
		names := make([]string, 0, len(commands))
		for n := range commands {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintln(os.Stderr, "usage: restic-backup <command> [args]")
		for _, n := range names {
			fmt.Fprintf(os.Stderr, "  %-16s %s\n", n, commands[n].help)
		}
		return 2
	}
	c, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		return 2
	}
	useLocalTimezone()
	return c.run(args[1:])
}
