package ipfsproxy

import "bytes"

// newMultiError creates a new MultiError object from a given slice of errors.
func newMultiError(errs ...error) *multiError {
	return &multiError{errs[:len(errs)-1], errs[len(errs)-1]}
}

// MultiError contains the results of multiple errors.
type multiError struct {
	Errors  []error
	Summary error
}

func (e *multiError) Error() string {
	var buf bytes.Buffer
	for _, err := range e.Errors {
		buf.WriteString(err.Error())
		buf.WriteString("; ")
	}
	buf.WriteString(e.Summary.Error())
	return buf.String()
}
