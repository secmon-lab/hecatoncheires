import { useState, useRef, useEffect } from 'react'
import { useQuery } from '@apollo/client'
import { ChevronDown, X, Search, Loader2, Hash } from 'lucide-react'
import { GET_SLACK_JOINED_CHANNELS } from '../../graphql/source'
import styles from './ChannelSelector.module.css'

interface Channel {
  id: string
  name: string
}

interface ChannelSelectorProps {
  selectedChannels: Channel[]
  onChange: (channels: Channel[]) => void
  disabled?: boolean
  error?: string
}

export default function ChannelSelector({
  selectedChannels,
  onChange,
  disabled,
  error,
}: ChannelSelectorProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)

  const { data, loading } = useQuery(GET_SLACK_JOINED_CHANNELS, {
    skip: !isOpen,
  })

  const availableChannels: Channel[] = data?.slackJoinedChannels || []

  const filteredChannels = availableChannels.filter(
    (channel) =>
      channel.name.toLowerCase().includes(searchQuery.toLowerCase()) &&
      !selectedChannels.some((selected) => selected.id === channel.id)
  )

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsOpen(false)
        setSearchQuery('')
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  useEffect(() => {
    if (isOpen && searchInputRef.current) {
      searchInputRef.current.focus()
    }
  }, [isOpen])

  const handleToggle = () => {
    if (!disabled) {
      setIsOpen(!isOpen)
      setSearchQuery('')
    }
  }

  const handleSelectChannel = (channel: Channel) => {
    onChange([...selectedChannels, channel])
    setSearchQuery('')
  }

  const handleRemoveChannel = (channelId: string) => {
    onChange(selectedChannels.filter((c) => c.id !== channelId))
  }

  return (
    <div className={styles.container} ref={containerRef}>
      <div
        className={`${styles.selector} ${isOpen ? styles.selectorOpen : ''} ${error ? styles.selectorError : ''} ${disabled ? styles.selectorDisabled : ''}`}
        onClick={handleToggle}
      >
        <div className={styles.selectedItems}>
          {selectedChannels.length === 0 ? (
            <span className={styles.placeholder}>Select channels...</span>
          ) : (
            selectedChannels.map((channel) => (
              <span key={channel.id} className={styles.tag}>
                <Hash size={12} />
                {channel.name}
                <button
                  type="button"
                  className={styles.tagRemove}
                  onClick={(e) => {
                    e.stopPropagation()
                    handleRemoveChannel(channel.id)
                  }}
                  disabled={disabled}
                >
                  <X size={14} />
                </button>
              </span>
            ))
          )}
        </div>
        <ChevronDown size={20} className={`${styles.chevron} ${isOpen ? styles.chevronOpen : ''}`} />
      </div>

      {isOpen && (
        <div className={styles.dropdown}>
          <div className={styles.searchContainer}>
            <Search size={16} className={styles.searchIcon} />
            <input
              ref={searchInputRef}
              type="text"
              className={styles.searchInput}
              placeholder="Search channels..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>

          <div className={styles.channelList}>
            {loading ? (
              <div className={styles.loading}>
                <Loader2 size={20} className={styles.spinner} />
                <span>Loading channels...</span>
              </div>
            ) : filteredChannels.length === 0 ? (
              <div className={styles.emptyState}>
                {searchQuery
                  ? 'No channels match your search'
                  : availableChannels.length === 0
                    ? 'No channels available. Make sure the bot is invited to channels.'
                    : 'All channels are already selected'}
              </div>
            ) : (
              filteredChannels.map((channel) => (
                <button
                  key={channel.id}
                  type="button"
                  className={styles.channelItem}
                  onClick={() => handleSelectChannel(channel)}
                >
                  <Hash size={16} className={styles.channelIcon} />
                  <span className={styles.channelName}>{channel.name}</span>
                </button>
              ))
            )}
          </div>
        </div>
      )}

      {error && <span className={styles.error}>{error}</span>}
    </div>
  )
}
