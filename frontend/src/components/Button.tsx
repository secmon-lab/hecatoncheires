import type { ButtonHTMLAttributes, ReactNode } from 'react'

type Variant = 'primary' | 'secondary' | 'danger' | 'outline' | 'ghost'
type Size = 'sm' | 'md' | 'lg'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
  icon?: ReactNode
  children?: ReactNode
}

const variantClass: Record<Variant, string> = {
  primary: 'primary',
  secondary: '',
  danger: 'danger',
  outline: '',
  ghost: 'ghost',
}

export default function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  children,
  className = '',
  type = 'button',
  ...rest
}: ButtonProps) {
  const classes = ['btn']
  const v = variantClass[variant]
  if (v) classes.push(v)
  if (size === 'sm') classes.push('sm')
  if (size === 'lg') classes.push('lg')
  if (!children) classes.push('icon')
  if (className) classes.push(className)
  return (
    <button type={type} className={classes.join(' ')} {...rest}>
      {icon}
      {children && <span>{children}</span>}
    </button>
  )
}
