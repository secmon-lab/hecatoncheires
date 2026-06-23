import { useRef, useState } from 'react'
import { commitOnEnter } from '../../utils/keyboard'
import { useTranslation } from '../../i18n'
import { IconX } from '../Icons'
import styles from './TagInput.module.css'

export interface TagOption {
  id: string
  name: string
}

interface TagInputProps {
  /** Selected tag IDs. */
  tags: string[]
  onChange: (tags: string[]) => void
  /** All available tags with their id and name. */
  availableTags?: TagOption[]
  /**
   * Called when the user types a name that matches no existing tag and
   * confirms. Should create the tag and return its new ID.
   */
  onCreateTag?: (name: string) => Promise<string>
  error?: boolean
  placeholder?: string
}

export default function TagInput({
  tags,
  onChange,
  availableTags = [],
  onCreateTag,
  error,
  placeholder,
}: TagInputProps) {
  const { t } = useTranslation()
  const [inputValue, setInputValue] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const nameById = (id: string): string => {
    return availableTags.find((t) => t.id === id)?.name ?? id
  }

  const addById = (id: string) => {
    if (!id || tags.includes(id)) {
      setInputValue('')
      return
    }
    onChange([...tags, id])
    setInputValue('')
  }

  const addByName = async (raw: string) => {
    const trimmed = raw.trim()
    if (!trimmed) {
      setInputValue('')
      return
    }
    // Check if a tag with this name already exists (case-insensitive)
    const existing = availableTags.find(
      (t) => t.name?.toLowerCase() === trimmed.toLowerCase(),
    )
    if (existing) {
      addById(existing.id)
      return
    }
    if (!onCreateTag) {
      setInputValue('')
      return
    }
    const newId = await onCreateTag(trimmed)
    addById(newId)
  }

  const removeTag = (id: string) => {
    onChange(tags.filter((t) => t !== id))
  }

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value
    // Comma immediately adds the tag
    if (val.endsWith(',')) {
      void addByName(val.slice(0, -1))
    } else {
      setInputValue(val)
    }
  }

  const handleKeyDown = commitOnEnter({
    onCommit: () => { void addByName(inputValue) },
  })

  // Suggestions: existing tags not yet selected, matching input prefix
  const filteredSuggestions = inputValue.trim()
    ? availableTags.filter(
        (t) =>
          !tags.includes(t.id) &&
          (t.name ?? '').toLowerCase().startsWith(inputValue.trim().toLowerCase()),
      )
    : []

  // Show "create" action when input doesn't exactly match an existing tag
  const showCreateAction =
    !!onCreateTag &&
    inputValue.trim().length > 0 &&
    !availableTags.some(
      (t) => (t.name ?? '').toLowerCase() === inputValue.trim().toLowerCase(),
    )

  const showDropdown = filteredSuggestions.length > 0 || showCreateAction

  return (
    <div className={`${styles.root} ${error ? styles.error : ''}`} onClick={() => inputRef.current?.focus()}>
      {tags.map((id) => (
        <span key={id} className="chip">
          {nameById(id)}
          <button
            type="button"
            className={styles.removeBtn}
            onClick={(e) => { e.stopPropagation(); removeTag(id) }}
            aria-label={`Remove ${nameById(id)}`}
          >
            <IconX size={10} />
          </button>
        </span>
      ))}
      <div className={styles.inputWrap}>
        <input
          ref={inputRef}
          type="text"
          className={styles.input}
          value={inputValue}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={tags.length === 0 ? (placeholder ?? t('placeholderKnowledgeTagInput')) : ''}
          data-testid="tag-input"
        />
        {showDropdown && (
          <ul className={styles.suggestions}>
            {filteredSuggestions.slice(0, 6).map((tag) => (
              <li
                key={tag.id}
                className={styles.suggestion}
                data-testid="tag-suggestion"
                onMouseDown={(e) => { e.preventDefault(); addById(tag.id) }}
              >
                {tag.name}
              </li>
            ))}
            {showCreateAction && (
              <li
                className={styles.suggestion}
                data-testid="tag-create-option"
                onMouseDown={(e) => {
                  e.preventDefault()
                  void addByName(inputValue)
                }}
              >
                {t('actionCreateTag', { name: inputValue.trim() })}
              </li>
            )}
          </ul>
        )}
      </div>
    </div>
  )
}
