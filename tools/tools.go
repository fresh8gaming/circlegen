//go:build tools
// +build tools

package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/kynrai/tainted"
	_ "gotest.tools/gotestsum"
)
