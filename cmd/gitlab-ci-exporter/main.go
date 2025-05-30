/*
2025 Helvethink <technical@helvethink.ch>
*/
package main

import (
	"github.com/helvethink/gitlab-ci-exporter/internal/cli"
	"os"
)

var version = "devel"

func main() {
	cli.Run(version, os.Args)
}
