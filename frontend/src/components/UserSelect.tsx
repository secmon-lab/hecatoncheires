import Select, { components, type GroupBase, type Props as SelectProps } from 'react-select'
import { Avatar } from './Primitives'
import { buildSelectStyles, portalProps } from './selectStyles'

export interface UserOption {
  value: string
  label: string
  name?: string | null
  realName?: string | null
  imageUrl?: string | null
}

const Option = (props: any) => (
  <components.Option {...props}>
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
      <Avatar
        size="sm"
        name={props.data.name || props.data.label}
        realName={props.data.realName || props.data.label}
        imageUrl={props.data.imageUrl}
      />
      <span>{props.data.label}</span>
    </span>
  </components.Option>
)

const SingleValue = (props: any) => (
  <components.SingleValue {...props}>
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
      <Avatar
        size="sm"
        name={props.data.name || props.data.label}
        realName={props.data.realName || props.data.label}
        imageUrl={props.data.imageUrl}
      />
      <span>{props.data.label}</span>
    </span>
  </components.SingleValue>
)

const MultiValueLabel = (props: any) => (
  <components.MultiValueLabel {...props}>
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
      <Avatar
        size="sm"
        name={props.data.name || props.data.label}
        realName={props.data.realName || props.data.label}
        imageUrl={props.data.imageUrl}
      />
      <span>{props.data.label}</span>
    </span>
  </components.MultiValueLabel>
)

export default function UserSelect<IsMulti extends boolean = false>(
  props: SelectProps<UserOption, IsMulti, GroupBase<UserOption>>,
) {
  return (
    <Select<UserOption, IsMulti, GroupBase<UserOption>>
      {...portalProps}
      styles={buildSelectStyles() as any}
      {...props}
      components={{ Option, SingleValue, MultiValueLabel, ...(props.components || {}) }}
    />
  )
}
