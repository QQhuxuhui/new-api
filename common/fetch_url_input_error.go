package common

import "errors"

// FetchURLInputError marks deterministic URL syntax or fetch-policy failures.
// Operational failures such as DNS lookup errors and invalid server-side fetch
// configuration must remain unmarked.
type FetchURLInputError struct {
	Err error
}

func (e *FetchURLInputError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *FetchURLInputError) Unwrap() error {
	return e.Err
}

func NewFetchURLInputError(err error) error {
	if err == nil || IsFetchURLInputError(err) {
		return err
	}
	return &FetchURLInputError{Err: err}
}

func IsFetchURLInputError(err error) bool {
	var target *FetchURLInputError
	return errors.As(err, &target)
}
