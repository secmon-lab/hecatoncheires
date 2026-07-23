const hexPattern = /^[0-9a-f]{32}$/

// notionHosts is the allow-list of hosts recognized as Notion web URLs.
// "notion.so" / "www.notion.so" are the classic hosts; "app.notion.com"
// is the newer host used by shared / copied page links.
// Kept in sync with pkg/domain/model/source.go (notionHosts).
const notionHosts = new Set(['notion.so', 'www.notion.so', 'app.notion.com'])

/**
 * Extracts a Notion database ID from either a raw ID or a Notion URL.
 * Returns the ID in UUID format (8-4-4-4-12) as required by the Notion API.
 *
 * Accepted formats:
 * - Raw ID: "abc123def456789012345678901234567"
 * - UUID format: "12345678-90ab-cdef-1234-567890abcdef"
 * - Notion URL: "https://www.notion.so/workspace/abc123def456...?v=..."
 * - Notion URL: "https://app.notion.com/p/workspace/Title-abc123def456..."
 */
export function parseNotionID(input: string): string | null {
  const trimmed = input.trim()
  if (!trimmed) return null

  let hex: string | null

  // The scheme is matched case-insensitively (URL schemes are not
  // case-sensitive), consistent with the backend parser.
  const lower = trimmed.toLowerCase()
  if (lower.startsWith('http://') || lower.startsWith('https://')) {
    hex = parseNotionURL(trimmed)
  } else {
    hex = normalizeNotionID(trimmed)
  }

  if (!hex) return null

  return toUUIDFormat(hex)
}

function parseNotionURL(raw: string): string | null {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return null
  }

  // Hostnames are case-insensitive; normalize before the allow-list check.
  const host = url.hostname.toLowerCase()
  if (!notionHosts.has(host)) {
    return null
  }

  const path = url.pathname.replace(/\/+$/, '')
  const segments = path.split('/')
  if (segments.length === 0) return null

  const lastSegment = segments[segments.length - 1]

  // Hex IDs are case-insensitive, so normalize to lower case (matching the
  // raw-ID path).
  const clean = lastSegment.replace(/-/g, '').toLowerCase()
  if (clean.length >= 32) {
    const candidate = clean.slice(-32)
    if (hexPattern.test(candidate)) {
      return candidate
    }
  }

  return null
}

function normalizeNotionID(input: string): string | null {
  const clean = input.replace(/-/g, '').toLowerCase()
  if (hexPattern.test(clean)) {
    return clean
  }
  return null
}

function toUUIDFormat(hex: string): string {
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20, 32)}`
}
