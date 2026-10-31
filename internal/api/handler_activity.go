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

// PlanRunsChanged gives the activity feed the Frontier runs updated since a time.
// Until Frontier's services have started, which they do on first use, there is
// nothing to read: starting them here would run their restart recovery early.
func (s *Server) PlanRunsChanged(since time.Time, limit int) ([]activity.PlanRun, error) {
	s.planningMu.RLock()
	started := s.planning != nil
	s.planningMu.RUnlock()
	if !started {
		return nil, nil
	}
	views, err := live.ChangedSince(s.store.GormDB(), since, limit)
	if err != nil {
		return nil, err
	}
	runs := make([]activity.PlanRun, 0, len(views))
	for _, view := range views {
		run := activity.PlanRun{ID: view.ID, PlanID: view.PlanID, ExchangeID: view.ExchangeID, Symbol: view.Plan.Snapshot.Symbol, UpdatedAt: view.UpdatedAt}
		for _, order := range view.Orders {
			run.Orders = append(run.Orders, activity.PlanOrder{
				ClientID: order.ClientID,
				OrderID:  order.OrderID,
				Role:     order.Role,
				Symbol:   order.Symbol,
				Quantity: string(order.Quantity),
				Status:   order.Status,
				Error:    order.Error,
			})
		}
		runs = append(runs, run)
	}
	return runs, nil
}
