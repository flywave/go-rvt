// rvtinfo prints identification metadata for a Revit .rvt file as JSON.
//
// Usage: rvtinfo <file.rvt>
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/flywave/go-rvt/rvt"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: rvtinfo <file.rvt>")
		os.Exit(2)
	}
	md, err := rvt.OpenFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(md); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
