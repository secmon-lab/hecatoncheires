import { useRef, useState } from 'react'
import { commitOnEnter } from '../../utils/keyboard'
import { useTranslation } from '../../i18n'
import { IconX } from '../Icons'
import styles from './TagInput.module.css'

interface TagInputProps {
  tags: string[]
  onChange: (tags: string[]) => void
  suggestions?: string[]
  error?: boolean
  placeholder?: string
}

export default function TagInput({ tags, onChange, suggestions = [], error, placeholder }: TagInputProps) {
  const { t } = useTranslation()
  const [inputValue, setInputValue] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const addTag = (raw: string) => {
    const trimmed = raw.trim()
    if (!trimmed || tags.includes(trimmed)) {
      setInputValue('')
      return
    }
    onChange([...tags, trimmed])
    setInputValue('')
  }

  const removeTag = (tag: string) => {
    onChange(tags.filter((t) => t !== tag))
  }

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value
    // Comma immediately adds the tag
    if (val.endsWith(',')) {
      addTag(val.slice(0, -1))
    } else {
      setInputValue(val)
    }
  }

  const handleKeyDown = commitOnEnter({
    onCommit: () => addTag(inputValue),
  })

  // Filtered suggestions: not already added and matching input prefix
  const filteredSuggestions = inputValue.trim()
    ? suggestions.filter(
        (s) => !tags.includes(s) && s.toLowerCase().startsWith(inputValue.trim().toLowerCase()),
      )
    : []

  return (
    <div className={`${styles.root} ${error ? styles.error : ''}`} onClick={() => inputRef.current?.focus()}>
      {tags.map((tag) => (
        <span key={tag} className="chip">
          {tag}
          <button
            type="button"
            className={styles.removeBtn}
            onClick={(e) => { e.stopPropagation(); removeTag(tag) }}
            aria-label={`Remove ${tag}`}
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
        {filteredSuggestions.length > 0 && (
          <ul className={styles.suggestions}>
            {filteredSuggestions.slice(0, 6).map((s) => (
              <li
                key={s}
                className={styles.suggestion}
                onMouseDown={(e) => { e.preventDefault(); addTag(s) }}
              >
                {s}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
