import { Copy, DollarSign, AlertCircle, Twitter, TrendingUp, BarChart3, Activity } from 'lucide-react'
import { useTranslation } from '../i18n'
import styles from './Dashboard.module.css'

export default function Dashboard() {
  const { t } = useTranslation()

  const topCards = [
    {
      label: t('dashUsedSpace'),
      value: '49/50',
      unit: 'GB',
      link: t('dashGetMoreSpace'),
      icon: Copy,
      gradient: 'linear-gradient(135deg, #DC2626 0%, #EF4444 100%)',
    },
    {
      label: t('dashRevenue'),
      value: '$34,245',
      link: t('dashLast24Hours'),
      icon: DollarSign,
      gradient: 'linear-gradient(135deg, #DC2626 0%, #F87171 100%)',
    },
    {
      label: t('dashFixedIssues'),
      value: '75',
      link: t('dashTrackedGithub'),
      icon: AlertCircle,
      gradient: 'linear-gradient(135deg, #B91C1C 0%, #DC2626 100%)',
    },
    {
      label: t('dashFollowers'),
      value: '+245',
      link: t('dashJustUpdated'),
      icon: Twitter,
      gradient: 'linear-gradient(135deg, #991B1B 0%, #B91C1C 100%)',
    },
  ]

  const chartCards = [
    {
      title: t('dashDailySales'),
      subtitle: t('dashDailySalesSubtitle'),
      gradient: 'linear-gradient(135deg, #66BB6A 0%, #43A047 100%)',
      icon: TrendingUp,
    },
    {
      title: t('dashEmailSubscriptions'),
      subtitle: t('dashLastCampaign'),
      gradient: 'linear-gradient(135deg, #FFA726 0%, #FB8C00 100%)',
      icon: BarChart3,
    },
    {
      title: t('dashCompletedTasks'),
      subtitle: t('dashLastCampaign'),
      gradient: 'linear-gradient(135deg, #EF5350 0%, #E53935 100%)',
      icon: Activity,
    },
  ]

  return (
    <div className={styles.container}>
      <div className={styles.topCardsGrid}>
        {topCards.map((card) => {
          const Icon = card.icon
          return (
            <div
              key={card.label}
              className={styles.topCard}
              style={{ background: card.gradient }}
            >
              <div className={styles.topCardIcon}>
                <Icon size={32} />
              </div>
              <div className={styles.topCardContent}>
                <p className={styles.topCardLabel}>{card.label}</p>
                <h3 className={styles.topCardValue}>
                  {card.value}
                  {card.unit && <span className={styles.unit}>{card.unit}</span>}
                </h3>
              </div>
              <div className={styles.topCardFooter}>
                <span className={styles.topCardLink}>{card.link}</span>
              </div>
            </div>
          )
        })}
      </div>

      <div className={styles.chartsGrid}>
        {chartCards.map((card) => {
          const Icon = card.icon
          return (
            <div key={card.title} className={styles.chartCard}>
              <div
                className={styles.chartHeader}
                style={{ background: card.gradient }}
              >
                <Icon size={24} className={styles.chartIcon} />
              </div>
              <div className={styles.chartContent}>
                <h3 className={styles.chartTitle}>{card.title}</h3>
                <p className={styles.chartSubtitle}>{card.subtitle}</p>
                <div className={styles.chartPlaceholder}>
                  {t('dashChartVisualization')}
                </div>
                <div className={styles.chartFooter}>
                  <span className={styles.chartTime}>{t('dashUpdatedAgo')}</span>
                </div>
              </div>
            </div>
          )
        })}
      </div>

      <div className={styles.bottomSection}>
        <div className={styles.tasksCard}>
          <div
            className={styles.tasksHeader}
            style={{ background: 'linear-gradient(135deg, #AB47BC 0%, #8E24AA 100%)' }}
          >
            <h3 className={styles.tasksTitle}>{t('dashTasks')}</h3>
            <div className={styles.tasksTabs}>
              <button className={`${styles.taskTab} ${styles.active}`}>{t('dashBugs')}</button>
              <button className={styles.taskTab}>{t('dashWebsite')}</button>
              <button className={styles.taskTab}>{t('dashServer')}</button>
            </div>
          </div>
          <div className={styles.tasksContent}>
            <div className={styles.taskItem}>
              <input type="checkbox" className={styles.checkbox} />
              <span>Sign contract for "What are conference organizers afraid of?"</span>
            </div>
            <div className={styles.taskItem}>
              <input type="checkbox" className={styles.checkbox} />
              <span>Lines From Great Russian Literature? Or E-mails From My Boss?</span>
            </div>
          </div>
        </div>

        <div className={styles.employeesCard}>
          <div
            className={styles.employeesHeader}
            style={{ background: 'linear-gradient(135deg, #FFA726 0%, #FB8C00 100%)' }}
          >
            <h3 className={styles.employeesTitle}>{t('dashEmployeesStats')}</h3>
            <p className={styles.employeesSubtitle}>{t('dashEmployeesSubtitle')}</p>
          </div>
          <div className={styles.employeesContent}>
            <div className={styles.employeesTable}>
              <div className={styles.employeesRow}>
                <span>{t('dashSalary')}</span>
                <span>{t('dashCountry')}</span>
                <span>{t('dashCity')}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
