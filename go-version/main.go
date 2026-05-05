package main

import (
	"os"

	"github.mf/manif3station/openvpn/go-version/mirror"
)

func main() {
	os.Exit(mirror.Run(os.Args[0], os.Args[1:], os.Stdin, os.Stdout, os.Stderr, nil))
}
