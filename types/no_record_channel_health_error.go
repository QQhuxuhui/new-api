package types

import "errors"

// NoRecordChannelHealthError marks a retryable local preparation failure that
// must not be attributed to the selected channel.
type NoRecordChannelHealthError struct {
	Err error
}

func (e *NoRecordChannelHealthError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *NoRecordChannelHealthError) Unwrap() error {
	return e.Err
}

func NewNoRecordChannelHealthError(err error) error {
	if err == nil {
		return nil
	}
	return &NoRecordChannelHealthError{Err: err}
}

func IsNoRecordChannelHealthError(err error) bool {
	var target *NoRecordChannelHealthError
	return errors.As(err, &target)
}
