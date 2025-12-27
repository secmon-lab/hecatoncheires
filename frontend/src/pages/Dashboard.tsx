import styles from './Dashboard.module.css'

function Dashboard() {
  return (
    <div className={styles.container}>
      <div className={styles.card}>
        <h1 className={styles.title}>
          Hecatoncheires
        </h1>
        <p className={styles.subtitle}>
          AI Native Risk Management System
        </p>
      </div>
    </div>
  )
}

export default Dashboard
