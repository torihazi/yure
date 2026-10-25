// yure shows earthquakes in Japan on a map in your terminal, live.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/torihazi/yure/internal/quake"
	"github.com/torihazi/yure/internal/ui"
)

// version is set by GoReleaser. For `go install`, the module version is used instead.
var version = ""

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	demo := flag.Bool("demo", false, "replay recent real earthquakes every few seconds instead of waiting for new ones")
	notifyMin := flag.String("notify", "3", `show a desktop notification for intensity at or above this (1-4, 5-, 5+, 6-, 6+, 7), or "off"`)
	flag.Parse()

	if *showVersion {
		fmt.Println("yure", versionString())
		return
	}

	opts := ui.Options{Demo: *demo}
	if *notifyMin != "off" {
		s, err := quake.ParseScale(*notifyMin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "yure: -notify:", err)
			os.Exit(2)
		}
		opts.NotifyMin = s
	}

	if err := ui.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "yure:", err)
		os.Exit(1)
	}
}

func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version // a pseudo-version when built from a checkout
	}
	return "unknown"
}
