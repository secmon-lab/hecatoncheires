import { useTranslation } from '../i18n'
import styles from './Top.module.css'

export default function Top() {
  const { t } = useTranslation()

  return (
    <div className={styles.container}>
      <div className={styles.logoContainer}>
        <img src="/logo-center.png" alt={t('appName')} className={styles.logo} />
      </div>
      <h1 className={styles.title}>{t('appName')}</h1>
      <p className={styles.subtitle}>{t('appSubtitle')}</p>
    </div>
  )
}
