import { Shield } from 'lucide-react'
import styles from './Top.module.css'

export default function Top() {
  return (
    <div className={styles.container}>
      <div className={styles.logoContainer}>
        <Shield size={80} className={styles.logo} />
      </div>
      <h1 className={styles.title}>Hecatoncheires</h1>
      <p className={styles.subtitle}>AI Native Risk Management System</p>
    </div>
  )
}
