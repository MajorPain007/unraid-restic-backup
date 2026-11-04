package main

import (
	"os"

	"github.com/MajorPain007/unraid-restic-backup/internal/rb"
)

func main() {
	os.Exit(rb.Main(os.Args[1:]))
}
