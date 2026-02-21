const hexPattern = /^[0-9a-f]{32}$/

/**
 * Extracts a Notion database ID from either a raw ID or a Notion URL.
 * Returns the ID in UUID format (8-4-4-4-12) as required by the Notion API.
 *
 * Accepted formats:
 * - Raw ID: "abc123def456789012345678901234567"
 * - UUID format: "12345678-90ab-cdef-1234-567890abcdef"
 * - Notion URL: "https://www.notion.so/workspace/abc123def456...?v=..."
 */
export function parseNotionID(input: string): string | null {
  const trimmed = input.trim()
  if (!trimmed) return null

  let hex: string | null

  if (trimmed.startsWith('http://') || trimmed.startsWith('https://')) {
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

  const host = url.hostname
  if (host !== 'www.notion.so' && host !== 'notion.so') {
    return null
  }

  const path = url.pathname.replace(/\/+$/, '')
  const segments = path.split('/')
  if (segments.length === 0) return null

  const lastSegment = segments[segments.length - 1]

  const clean = lastSegment.replace(/-/g, '')
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
