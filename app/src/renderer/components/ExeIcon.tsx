import { ProgramIcon, ServicesIcon } from '../icons/builtin'
import { useIcon } from '../services'
import styles from './ExeIcon.module.css'

/** The icon of a program's file, or a built-in one while it loads, when there is none, and for the services tile. */
export function ExeIcon({ exe, system = false }: { exe: string; system?: boolean }): React.JSX.Element {
  const url = useIcon(system ? '' : exe)
  if (url !== null) return <img className={styles.icon} src={url} alt="" draggable={false} />
  const Builtin = system ? ServicesIcon : ProgramIcon
  return (
    <span className={styles.icon} data-builtin="">
      <Builtin />
    </span>
  )
}
