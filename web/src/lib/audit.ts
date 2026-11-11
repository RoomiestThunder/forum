import type { AuditEntry } from './types'

function roleName(isAdmin: unknown, isModerator: unknown): string {
  if (isAdmin) return 'Admin'
  if (isModerator) return 'Moderator'
  return 'Member'
}

function quoted(value: unknown): string {
  return typeof value === 'string' && value !== '' ? `"${value}"` : 'a post'
}

/** One-line, human-readable summary of an audit entry (without the actor). */
export function describeAudit(entry: AuditEntry): string {
  const d = entry.details
  switch (entry.action) {
    case 'post.pin':
      return `pinned ${quoted(d.title)}`
    case 'post.unpin':
      return `unpinned ${quoted(d.title)}`
    case 'post.delete':
      return `deleted ${quoted(d.title)} by ${String(d.author ?? 'unknown')}`
    case 'comment.delete':
      return `deleted a comment by ${String(d.author ?? 'unknown')}`
    case 'user.ban':
      return `banned ${String(d.username ?? `user #${entry.target_id}`)}`
    case 'user.unban':
      return `unbanned ${String(d.username ?? `user #${entry.target_id}`)}`
    case 'user.role':
      return `changed ${String(d.username ?? `user #${entry.target_id}`)} from ${roleName(d.was_admin, d.was_moderator)} to ${roleName(d.is_admin, d.is_moderator)}`
    case 'report.resolve':
      return `resolved report #${entry.target_id}`
    default:
      // An action this client doesn't know (e.g. added by a newer server) is
      // still shown rather than hidden.
      return `${entry.action} on ${entry.target_type} #${entry.target_id}`
  }
}

/** The text a delete removed, when the entry kept it. */
export function auditExcerpt(entry: AuditEntry): string | null {
  const excerpt = entry.details.excerpt
  return typeof excerpt === 'string' && excerpt !== '' ? excerpt : null
}
