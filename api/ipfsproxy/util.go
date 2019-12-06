package ipfsproxy

import (
	"strings"
)

// MultiError contains the results of multiple errors.
type multiError struct {
	err strings.Builder
}

func (e *multiError) add(err string) {
	e.err.WriteString(err)
	e.err.WriteString("; ")
}

func (e *multiError) Error() string {
	return e.err.String()
}
