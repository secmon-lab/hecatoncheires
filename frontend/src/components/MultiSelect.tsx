import { useState, useRef, useEffect } from 'react'
import { createPortal } from 'react-dom'
import Chip from './Chip'
import styles from './MultiSelect.module.css'

export interface MultiSelectOption {
  value: string
  label: string
  icon?: React.ReactNode
}

interface MultiSelectProps {
  options: MultiSelectOption[]
  value: string[]
  onChange: (value: string[]) => void
  placeholder?: string
  variant?: 'default' | 'category' | 'team' | 'user'
  disabled?: boolean
  searchable?: boolean
}

export default function MultiSelect({
  options,
  value,
  onChange,
  placeholder = 'Select items...',
  variant = 'default',
  disabled = false,
  searchable = true,
}: MultiSelectProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [dropdownPosition, setDropdownPosition] = useState({ top: 0, left: 0, width: 0 })
  const containerRef = useRef<HTMLDivElement>(null)

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsOpen(false)
        setSearchQuery('')
      }
    }

    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [isOpen])

  // Update dropdown position
  const updatePosition = () => {
    if (containerRef.current) {
      const rect = containerRef.current.getBoundingClientRect()
      setDropdownPosition({
        top: rect.bottom + 4,
        left: rect.left,
        width: rect.width,
      })
    }
  }

  // Update position when opening and on scroll/resize
  useEffect(() => {
    if (isOpen) {
      updatePosition()
      window.addEventListener('scroll', updatePosition, true)
      window.addEventListener('resize', updatePosition)
      return () => {
        window.removeEventListener('scroll', updatePosition, true)
        window.removeEventListener('resize', updatePosition)
      }
    }
  }, [isOpen])

  // Filter options based on search query
  const filteredOptions = options.filter(option =>
    option.label.toLowerCase().includes(searchQuery.toLowerCase())
  )

  // Get selected options
  const selectedOptions = options.filter(option => value.includes(option.value))

  const handleToggleOption = (optionValue: string) => {
    if (value.includes(optionValue)) {
      onChange(value.filter(v => v !== optionValue))
    } else {
      onChange([...value, optionValue])
    }
  }

  const handleRemoveOption = (optionValue: string) => {
    onChange(value.filter(v => v !== optionValue))
  }

  const handleToggle = () => {
    if (!disabled) {
      const newIsOpen = !isOpen
      setIsOpen(newIsOpen)
      if (!newIsOpen) {
        setSearchQuery('')
      }
    }
  }

  return (
    <>
      <div className={styles.container} ref={containerRef}>
        <div
          className={`${styles.control} ${isOpen ? styles.open : ''} ${disabled ? styles.disabled : ''}`}
          onClick={handleToggle}
        >
          <div className={styles.valueContainer}>
            {selectedOptions.length > 0 ? (
              <div className={styles.chips}>
                {selectedOptions.map(option => (
                  <Chip
                    key={option.value}
                    variant={variant}
                    size="sm"
                    icon={option.icon}
                    onRemove={disabled ? undefined : () => {
                      handleRemoveOption(option.value)
                    }}
                  >
                    {option.label}
                  </Chip>
                ))}
              </div>
            ) : null}
            {searchable && isOpen && (
              <input
                type="text"
                className={styles.input}
                placeholder={selectedOptions.length === 0 ? placeholder : 'Search...'}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                onClick={(e) => e.stopPropagation()}
                autoFocus
                disabled={disabled}
              />
            )}
            {(!isOpen || !searchable) && selectedOptions.length === 0 && (
              <span className={styles.placeholder}>{placeholder}</span>
            )}
          </div>
          <button
            type="button"
            className={styles.toggleButton}
            onClick={(e) => {
              e.stopPropagation()
              handleToggle()
            }}
            disabled={disabled}
            aria-label="Toggle options"
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 20 20"
              fill="currentColor"
              style={{ transform: isOpen ? 'rotate(180deg)' : 'rotate(0deg)', transition: 'transform 0.2s' }}
            >
              <path d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" />
            </svg>
          </button>
        </div>
      </div>

      {isOpen && !disabled && createPortal(
        <div
          className={styles.dropdown}
          style={{
            position: 'fixed',
            top: `${dropdownPosition.top}px`,
            left: `${dropdownPosition.left}px`,
            width: `${dropdownPosition.width}px`,
            zIndex: 10000,
          }}
        >
          {filteredOptions.length > 0 ? (
            <ul className={styles.optionList}>
              {filteredOptions.map(option => {
                const isSelected = value.includes(option.value)
                return (
                  <li
                    key={option.value}
                    className={`${styles.option} ${isSelected ? styles.selected : ''}`}
                    onClick={(e) => {
                      e.stopPropagation()
                      handleToggleOption(option.value)
                    }}
                  >
                    <div className={styles.checkbox}>
                      {isSelected && (
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
                          <path d="M13.854 3.646a.5.5 0 010 .708l-7 7a.5.5 0 01-.708 0l-3.5-3.5a.5.5 0 11.708-.708L6.5 10.293l6.646-6.647a.5.5 0 01.708 0z" />
                        </svg>
                      )}
                    </div>
                    {option.icon && <span className={styles.optionIcon}>{option.icon}</span>}
                    <span className={styles.optionLabel}>{option.label}</span>
                  </li>
                )
              })}
            </ul>
          ) : (
            <div className={styles.noOptions}>No options found</div>
          )}
        </div>,
        document.body
      )}
    </>
  )
}
