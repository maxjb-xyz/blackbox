import type { NotificationPolicy } from '../api/client'

const HHMM_RE = /^([01]\d|2[0-3]):[0-5]\d$/

// validatePolicyForm mirrors the server-side validatePolicy rules, returning an
// error message or null when the policy is acceptable. Lives in its own pure
// module (no JSX) so it can be unit-tested under the Node test runner.
export function validatePolicyForm(p: NotificationPolicy): string | null {
  if (p.quiet_hours_enabled) {
    if (!HHMM_RE.test(p.quiet_hours_start) || !HHMM_RE.test(p.quiet_hours_end)) {
      return 'Quiet hours start/end must be HH:MM (24h)'
    }
    if (p.quiet_hours_mode !== 'drop' && p.quiet_hours_mode !== 'defer') {
      return 'Quiet hours mode must be drop or defer'
    }
  }
  if (p.rate_limit_enabled) {
    if (!Number.isFinite(p.rate_limit_count) || p.rate_limit_count < 1) {
      return 'Rate limit count must be at least 1'
    }
    if (p.rate_limit_unit !== 'hour' && p.rate_limit_unit !== 'day') {
      return 'Rate limit unit must be hour or day'
    }
  }
  return null
}
