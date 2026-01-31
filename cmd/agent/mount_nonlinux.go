//go:build !linux

package main

import "fmt"

func mountCodeDrive() {
	fmt.Println("[agent] Non-linux platform, skipping code drive mount")
}
