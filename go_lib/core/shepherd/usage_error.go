package shepherd

import "errors"

type UsageError struct {
	err   error
	usage *Usage
}

func newUsageError(err error, usage *Usage) error {
	if err == nil {
		return nil
	}
	if normalizeUsage(usage) == nil {
		return err
	}
	return &UsageError{err: err, usage: mergeUsage(usage, nil)}
}

func (e *UsageError) Error() string {
	return e.err.Error()
}

func (e *UsageError) Unwrap() error {
	return e.err
}

func (e *UsageError) Usage() *Usage {
	return mergeUsage(e.usage, nil)
}

func UsageFromError(err error) *Usage {
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return usageErr.Usage()
	}
	return nil
}
