package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"unicode/utf8"

	"forum/internal/models"
)

// The actions recorded in the audit log. These strings are stored and shown in
// the admin panel, so changing one orphans the history recorded under it.
const (
	auditPostDelete    = "post.delete"
	auditPostPin       = "post.pin"
	auditPostUnpin     = "post.unpin"
	auditCommentDelete = "comment.delete"
	auditUserBan       = "user.ban"
	auditUserUnban     = "user.unban"
	auditUserRole      = "user.role"
	auditReportResolve = "report.resolve"
)

// excerptLength bounds how much of a deleted post or comment is kept in the
// log: enough to see what was removed and why it might have been reported,
// without copying whole posts into a second table.
const excerptLength = 200

// audit records an action taken by staff. It runs after the action has
// succeeded, and a failure to record it is logged rather than returned: the
// action already happened, and telling the moderator it failed would invite a
// retry of something that can't be retried. details is a snapshot of what was
// acted on, since for a delete the thing itself is gone afterwards.
func (s *Server) audit(r *http.Request, action, targetType string, targetID int, details map[string]interface{}) {
	if details == nil {
		details = map[string]interface{}{}
	}
	data, err := json.Marshal(details)
	if err == nil {
		err = s.store.CreateAuditEntry(userIDFromCtx(r), action, targetType, targetID, string(data))
	}
	if err != nil {
		s.logger.Error("audit log write failed",
			slog.String("action", action),
			slog.String("target_type", targetType),
			slog.Int("target_id", targetID),
			slog.Int("actor_id", userIDFromCtx(r)),
			slog.Any("err", err),
		)
	}
}

// excerpt shortens text to excerptLength characters, marking the cut.
func excerpt(text string) string {
	if utf8.RuneCountInString(text) <= excerptLength {
		return text
	}
	return string([]rune(text)[:excerptLength]) + "…"
}

// listAudit serves the audit log, newest first. Admin only: it records what
// moderators did, including actions against other staff.
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const pageSize = 50
	entries, err := s.store.ListAuditEntries(pageSize, (page-1)*pageSize)
	if err != nil {
		s.storeError(w, r, err, "load audit log")
		return
	}
	total, err := s.store.CountAuditEntries()
	if err != nil {
		s.storeError(w, r, err, "load audit log")
		return
	}
	if entries == nil {
		entries = []*models.AuditEntry{} // serialise as [], not null
	}
	jsonResponse(w, map[string]interface{}{"entries": entries, "total": total, "page": page}, http.StatusOK)
}
