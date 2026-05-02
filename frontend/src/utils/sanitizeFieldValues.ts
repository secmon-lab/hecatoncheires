interface FieldOption {
  id: string
  name: string
}

interface FieldDef {
  id: string
  type: string
  options?: FieldOption[]
}

interface FieldValueInput {
  fieldId: string
  value: any
}

// Drop SELECT/MULTI_SELECT values whose option ID is no longer present in the
// current field configuration. Without this, editing a Case whose stored
// option ID was removed (config drift) makes Save fail server-side because
// the validator rejects unknown option IDs.
export function sanitizeFieldValues(
  values: FieldValueInput[],
  defs: FieldDef[],
): FieldValueInput[] {
  const defByID = new Map<string, FieldDef>()
  defs.forEach((d) => defByID.set(d.id, d))
  const out: FieldValueInput[] = []
  for (const fv of values) {
    const def = defByID.get(fv.fieldId)
    if (!def) {
      out.push(fv)
      continue
    }
    if (def.type === 'SELECT') {
      const ok = (def.options || []).some((o) => o.id === fv.value)
      if (!ok) continue
      out.push(fv)
    } else if (def.type === 'MULTI_SELECT') {
      const ids = Array.isArray(fv.value) ? fv.value : []
      const filtered = ids.filter((id: string) =>
        (def.options || []).some((o) => o.id === id),
      )
      if (filtered.length === 0) continue
      out.push({ fieldId: fv.fieldId, value: filtered })
    } else {
      out.push(fv)
    }
  }
  return out
}
