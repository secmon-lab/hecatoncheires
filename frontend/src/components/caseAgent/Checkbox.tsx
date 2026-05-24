import styles from '../../pages/CaseAgent.module.css'
import { IconCheck } from '../Icons'

interface Props {
  checked: boolean
}

// Decorative checkbox used in source rows and lists. Real interactivity
// is owned by the surrounding clickable row; the box itself is purely
// visual so it never steals the click target.
export default function Checkbox({ checked }: Props) {
  return (
    <span className={[styles.checkbox, checked ? styles.checked : ''].join(' ')} aria-hidden>
      {checked && <IconCheck size={10} sw={3} />}
    </span>
  )
}
