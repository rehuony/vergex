package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"vergex/internal/activity"
	"vergex/internal/planning/live"

	"github.com/gin-gonic/gin"
)

// activityTotalsEvery keeps a route's last hour true when nothing new happens, and
// shows the client the connection is alive.
const activityTotalsEvery = 20 * time.Second

type activityMessage struct {
	Type string `json:"type"`
	activity.Update
}

func (s *Server) handleActivityStream(c *gin.Context) {
	if s.activity == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Activity is unavailable"})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	send := func(kind string, update activity.Update) error {
		body, err := json.Marshal(activityMessage{Type: kind, Update: update})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", body); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	wake, stop := s.activity.Subscribe()
	defer stop()
	update := s.activity.After(0)
	if send("snapshot", update) != nil {
		return
	}
	seq := update.Seq
	ticker := time.NewTicker(activityTotalsEvery)
	defer ticker.Stop()
	for {
		kind := "events"
		select {
		case <-c.Request.Context().Done():
			return
		case <-wake:
		case <-ticker.C:
			kind = "totals"
		}
		update := s.activity.After(seq)
		seq = update.Seq
		if len(update.Events) > 0 {
			kind = "events"
		}
		if send(kind, update) != nil {
			return
		}
	}
}

// RefusedPlanOrders gives the activity feed the Frontier orders refused since a time.
// It only reads the database, so Frontier's services need not have started.
func (s *Server) RefusedPlanOrders(since time.Time, limit int) ([]activity.RefusedOrder, error) {
	rows, err := live.RefusedSince(s.store, since, limit)
	if err != nil {
		return nil, err
	}
	orders := make([]activity.RefusedOrder, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, activity.RefusedOrder{
			RunID:     row.RunID,
			PlanID:    row.PlanID,
			AccountID: row.AccountID,
			ClientID:  row.ClientID,
			Role:      row.Role,
			Symbol:    row.Symbol,
			Quantity:  row.Quantity,
			Error:     row.Error,
			At:        row.UpdatedAt,
		})
	}
	return orders, nil
}
