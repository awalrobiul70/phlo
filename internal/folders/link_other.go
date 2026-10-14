//go:build !windows

package folders

import "fmt"

func junction(link, _ string) error {
	return fmt.Errorf("cannot recreate the link %s", link)
}
