package zfs

import (
	"fmt"
)

type CommandError struct {
	Args   []string
	Cause  error
	Stderr string
}

func (m *CommandError) Error() string {
	return fmt.Sprintf("%s failed with %v: %s", m.Args, m.Cause, m.Stderr)
}
