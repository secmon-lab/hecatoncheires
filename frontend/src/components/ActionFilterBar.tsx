import { Search, X } from 'lucide-react'
import Select from 'react-select'
import { useQuery } from '@apollo/client'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import styles from './ActionFilterBar.module.css'

interface ActionFilterBarProps {
  searchText: string
  onSearchTextChange: (text: string) => void
  selectedAssigneeIDs: string[]
  onAssigneeChange: (ids: string[]) => void
}

export default function ActionFilterBar({
  searchText,
  onSearchTextChange,
  selectedAssigneeIDs,
  onAssigneeChange,
}: ActionFilterBarProps) {
  const { data: usersData } = useQuery(GET_SLACK_USERS)

  const assigneeOptions = (usersData?.slackUsers || []).map(
    (user: { id: string; name: string; realName: string; imageUrl?: string }) => ({
      value: user.id,
      label: user.realName || user.name,
      image: user.imageUrl,
    })
  )

  const selectedAssignees = assigneeOptions.filter((opt: { value: string }) =>
    selectedAssigneeIDs.includes(opt.value)
  )

  const hasFilters = searchText !== '' || selectedAssigneeIDs.length > 0

  return (
    <div className={styles.filterBar} data-testid="action-filter-bar">
      <div className={styles.searchWrapper}>
        <Search size={16} className={styles.searchIcon} />
        <input
          type="text"
          className={styles.searchInput}
          placeholder="Search by title, description, or case name..."
          value={searchText}
          onChange={(e) => onSearchTextChange(e.target.value)}
          data-testid="action-search-input"
        />
      </div>
      <div className={styles.assigneeWrapper}>
        <Select
          isMulti
          isClearable
          value={selectedAssignees}
          onChange={(selected) => {
            onAssigneeChange([...(selected || [])].map((s) => s.value))
          }}
          options={assigneeOptions}
          placeholder="Filter by assignee..."
          formatOptionLabel={(option: { value: string; label: string; image?: string }) => (
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              {option.image && (
                <img
                  src={option.image}
                  alt={option.label}
                  style={{ width: '1.25rem', height: '1.25rem', borderRadius: '50%' }}
                />
              )}
              <span>{option.label}</span>
            </div>
          )}
          classNamePrefix="assignee-select"
          data-testid="action-assignee-filter"
        />
      </div>
      {hasFilters && (
        <button
          className={styles.clearButton}
          onClick={() => {
            onSearchTextChange('')
            onAssigneeChange([])
          }}
          data-testid="action-filter-clear"
        >
          <X size={14} />
          Clear
        </button>
      )}
    </div>
  )
}
