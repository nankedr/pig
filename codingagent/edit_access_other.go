//go:build !unix

package codingagent

import "os"

func editFileAccess(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return file.Close()
}
