package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gabinante/flywheel/internal/activity"
)

type ActivityHandler struct{ Hub *activity.Hub }

func (h *ActivityHandler) stream(w http.ResponseWriter, r *http.Request) {
	ch, unsubscribe := h.Hub.Subscribe()
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	write := func(data string) error {
		// Bound slow-client writes without applying a lifetime timeout to SSE.
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprint(w, data); err != nil {
			return err
		}
		return controller.Flush()
	}
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	if err := write("retry: 2000\n\n"); err != nil {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(e)
			if err := write(fmt.Sprintf("event: activity\ndata: %s\n\n", data)); err != nil {
				return
			}
		case <-ticker.C:
			if err := write("event: heartbeat\ndata: {}\n\n"); err != nil {
				return
			}
		}
	}
}
