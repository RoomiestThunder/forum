package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// notify persists a notification for recipientID and, best-effort, pushes a
// live signal over the websocket hub. The hub payload is intentionally
// sparse (just enough to identify what changed) - clients react to it by
// refetching the notification list/count rather than trusting the socket
// message to carry the full, correctly-shaped record.
//
// actorID is who caused the event. If the recipient has blocked them, the
// notification is dropped: a block that still let the blocked user ping you
// with mentions and likes would be only half a block.
func (s *Server) notify(recipientID, actorID int, notifType string, payload map[string]interface{}) {
	if blocked, err := s.store.IsBlocked(recipientID, actorID); err == nil && blocked {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if _, err := s.store.CreateNotification(recipientID, notifType, string(data)); err != nil {
		return
	}
	if s.hub != nil {
		s.hub.Notify(recipientID, notifType, payload)
	}
}

type notificationDTO struct {
	ID        int             `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Read      bool            `json:"read"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const pageSize = 20
	records, err := s.store.GetNotificationsByUser(userID, pageSize, (page-1)*pageSize)
	if err != nil {
		jsonError(w, "failed to load notifications", http.StatusInternalServerError)
		return
	}
	out := make([]notificationDTO, len(records))
	for i, n := range records {
		out[i] = notificationDTO{
			ID:        n.ID,
			Type:      n.Type,
			Payload:   json.RawMessage(n.Payload),
			Read:      n.Read,
			CreatedAt: n.CreatedAt,
		}
	}
	jsonResponse(w, out, http.StatusOK)
}

func (s *Server) unreadNotificationCount(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	count, err := s.store.CountUnreadNotifications(userID)
	if err != nil {
		jsonError(w, "failed to load count", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]int{"count": count}, http.StatusOK)
}

func (s *Server) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid notification id", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.MarkNotificationRead(id, userID); err != nil {
		jsonError(w, "failed to update notification", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) markAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	if err := s.store.MarkAllNotificationsRead(userID); err != nil {
		jsonError(w, "failed to update notifications", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}
