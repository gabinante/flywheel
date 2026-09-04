package rest

import (
	"encoding/json"
	"fmt"

	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// convertByJSON round-trips src through JSON into T. Used to map loosely typed
// request bodies (map[string]any) onto internal config structs.
func convertByJSON[T any](src any) (T, error) {
	var dst T
	raw, err := json.Marshal(src)
	if err != nil {
		return dst, apierrors.New(apierrors.CodeInternal, fmt.Sprintf("marshal %T: %v", src, err), true)
	}
	if err := json.Unmarshal(raw, &dst); err != nil {
		return dst, apierrors.New(apierrors.CodeInternal, fmt.Sprintf("unmarshal into %T: %v", dst, err), true)
	}
	return dst, nil
}
