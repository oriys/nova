//go:build !linux

package main

import "fmt"

func mountCodeDrive() {
	fmt.Println("[agent] Non-linux platform, skipping code drive mount")
}

func remountCodeDriveRW() error { return nil }
func remountCodeDriveRO() error { return nil }

func mountLayerDrives() {
	fmt.Println("[agent] Non-linux platform, skipping layer drive mount")
}

func mountLayerOverlay(layerCount int) {}

func mountVolumeDrives(layerCount int, mounts []VolumeMountInfo) {}
