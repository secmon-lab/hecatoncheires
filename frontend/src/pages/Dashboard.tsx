import { Copy, DollarSign, AlertCircle, Twitter, TrendingUp, BarChart3, Activity } from 'lucide-react'
import styles from './Dashboard.module.css'

export default function Dashboard() {
  const topCards = [
    {
      label: 'Used Space',
      value: '49/50',
      unit: 'GB',
      link: 'Get More Space',
      icon: Copy,
      gradient: 'linear-gradient(135deg, #FFA726 0%, #FB8C00 100%)',
    },
    {
      label: 'Revenue',
      value: '$34,245',
      link: 'Last 24 Hours',
      icon: DollarSign,
      gradient: 'linear-gradient(135deg, #66BB6A 0%, #43A047 100%)',
    },
    {
      label: 'Fixed Issues',
      value: '75',
      link: 'Tracked from Github',
      icon: AlertCircle,
      gradient: 'linear-gradient(135deg, #EF5350 0%, #E53935 100%)',
    },
    {
      label: 'Followers',
      value: '+245',
      link: 'Just Updated',
      icon: Twitter,
      gradient: 'linear-gradient(135deg, #26C6DA 0%, #00ACC1 100%)',
    },
  ]

  const chartCards = [
    {
      title: 'Daily Sales',
      subtitle: '55% increase in today sales',
      gradient: 'linear-gradient(135deg, #66BB6A 0%, #43A047 100%)',
      icon: TrendingUp,
    },
    {
      title: 'Email Subscriptions',
      subtitle: 'Last Campaign Performance',
      gradient: 'linear-gradient(135deg, #FFA726 0%, #FB8C00 100%)',
      icon: BarChart3,
    },
    {
      title: 'Completed Tasks',
      subtitle: 'Last Campaign Performance',
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
                  Chart visualization
                </div>
                <div className={styles.chartFooter}>
                  <span className={styles.chartTime}>updated 4 minutes ago</span>
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
            <h3 className={styles.tasksTitle}>Tasks:</h3>
            <div className={styles.tasksTabs}>
              <button className={`${styles.taskTab} ${styles.active}`}>BUGS</button>
              <button className={styles.taskTab}>WEBSITE</button>
              <button className={styles.taskTab}>SERVER</button>
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
            <h3 className={styles.employeesTitle}>Employees Stats</h3>
            <p className={styles.employeesSubtitle}>New employees on 15th September, 2016</p>
          </div>
          <div className={styles.employeesContent}>
            <div className={styles.employeesTable}>
              <div className={styles.employeesRow}>
                <span>Salary</span>
                <span>Country</span>
                <span>City</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
