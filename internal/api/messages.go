package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	convos, err := s.store.ListConversations(userID)
	if err != nil {
		jsonError(w, "failed to load conversations", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, convos, http.StatusOK)
}

func (s *Server) unreadMessageCount(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	count, err := s.store.CountUnreadMessages(userID)
	if err != nil {
		jsonError(w, "failed to load count", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]int{"count": count}, http.StatusOK)
}

// otherParticipant resolves the {username} route param to a user, rejecting
// self-conversations. The conversation itself is always derived from
// (caller, other) rather than trusted from client input, so there's no
// conversation id for a client to forge access to someone else's thread.
func (s *Server) otherParticipant(w http.ResponseWriter, r *http.Request) (id int, ok bool) {
	username := chi.URLParam(r, "username")
	other, err := s.store.GetUserByUsername(username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return 0, false
	}
	if other.ID == userIDFromCtx(r) {
		jsonError(w, "cannot message yourself", http.StatusBadRequest)
		return 0, false
	}
	return other.ID, true
}

func (s *Server) getConversationWithUser(w http.ResponseWriter, r *http.Request) {
	otherID, ok := s.otherParticipant(w, r)
	if !ok {
		return
	}
	userID := userIDFromCtx(r)
	convID, err := s.store.GetOrCreateConversation(userID, otherID)
	if err != nil {
		jsonError(w, "failed to load conversation", http.StatusInternalServerError)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const pageSize = 50
	messages, err := s.store.GetMessages(convID, pageSize, (page-1)*pageSize)
	if err != nil {
		jsonError(w, "failed to load messages", http.StatusInternalServerError)
		return
	}
	_ = s.store.MarkConversationRead(convID, userID)
	jsonResponse(w, map[string]interface{}{"messages": messages}, http.StatusOK)
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	otherID, ok := s.otherParticipant(w, r)
	if !ok {
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" || len([]rune(body.Content)) > 2000 {
		jsonError(w, "content must be 1-2000 chars", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	convID, err := s.store.GetOrCreateConversation(userID, otherID)
	if err != nil {
		jsonError(w, "failed to load conversation", http.StatusInternalServerError)
		return
	}
	msg, err := s.store.CreateMessage(convID, userID, body.Content)
	if err != nil {
		jsonError(w, "failed to send message", http.StatusInternalServerError)
		return
	}
	if s.hub != nil {
		s.hub.Notify(otherID, "new_message", map[string]interface{}{
			"conversation_id": convID,
			"sender":          usernameFromCtx(r),
			"preview":         body.Content,
		})
	}
	jsonResponse(w, msg, http.StatusCreated)
}
