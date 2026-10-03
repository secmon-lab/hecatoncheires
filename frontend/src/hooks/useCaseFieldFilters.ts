import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router'
import { clearCaseFieldFilters, readCaseFieldFilters, writeCaseFieldFilter } from '../utils/caseFieldFilters'

// The URL is the source of truth, including after reload, browser Back/Forward
// and external links. Match the list's status/page convention: replace history
// and preserve parameters owned by other controls.
export function useCaseFieldFilters() {
  const [params, setParams] = useSearchParams()
  const filters = useMemo(() => readCaseFieldFilters(params), [params])
  const onChange = useCallback((id: string, values: readonly string[]) => {
    setParams((prev) => writeCaseFieldFilter(prev, id, values), { replace: true })
  }, [setParams])
  const onClear = useCallback(() => {
    setParams((prev) => clearCaseFieldFilters(prev), { replace: true })
  }, [setParams])
  return { filters, onChange, onClear }
}
