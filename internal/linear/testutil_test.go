package linear

import (
	"errors"
	"time"
)

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func errorsIs(err, target error) bool { return errors.Is(err, target) }
