import InlineText from './InlineText'

interface Props {
  value: string
  onSave: (next: string) => Promise<void> | void
  ariaLabel: string
  placeholder?: string
  disabled?: boolean
  testId?: string
}

// URL is edited as plain text. The display layer (CustomFieldRenderer.display)
// is responsible for rendering it as a link; in inline mode we want the value
// to be editable, so we just show the raw URL.
export default function InlineURL(props: Props) {
  return <InlineText {...props} allowEmpty />
}
