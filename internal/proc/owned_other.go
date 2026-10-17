//go:build !windows

package proc

import (
	"os"
	"strconv"
)

// OwnedServicePIDs lists service programs started from this Phlo folder.
func OwnedServicePIDs(root string) []int {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		image, err := os.Readlink("/proc/" + entry.Name() + "/exe")
		if err != nil {
			continue
		}
		if IsPhloService(root, image) {
			pids = append(pids, pid)
		}
	}
	return pids
}
