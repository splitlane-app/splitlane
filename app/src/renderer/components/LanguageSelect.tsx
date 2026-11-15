import { useId } from 'react'
import { isLanguageSetting, type Lang, type LanguageSetting } from '../../shared/i18n'
import type { Key } from '../i18n/ru'
import { t } from '../i18n/t'
import { useApp, useServices } from '../services'
import styles from '../screens/screens.module.css'

/** The choices of «Язык»: the names of the languages in their own language (the `lang` of the option says which). */
const LANGUAGE_CHOICES: readonly { value: LanguageSetting; text: Key; lang?: Lang }[] = [
  { value: 'system', text: 'connection.languageSystem' },
  { value: 'ru', text: 'language.ru', lang: 'ru' },
  { value: 'en', text: 'language.en', lang: 'en' }
]

/**
 * «Язык» of the window: the setting goes to the main process, the page follows the language of its next snapshot.
 * Shown on the setup, connect and connection screens, so the language can be changed before the board.
 */
export function LanguageSelect(): React.JSX.Element | null {
  const { bridge, store } = useServices()
  const value = useApp((s) => s.snapshot?.settings.language ?? null)
  const id = useId()
  if (value === null) return null
  return (
    <div className={styles.choice}>
      <label className={styles.label} htmlFor={id}>
        {t('connection.language')}
      </label>
      <select
        id={id}
        className={styles.select}
        value={value}
        onChange={(e) => {
          const next = e.target.value
          if (!isLanguageSetting(next)) return
          bridge.setSettings({ language: next }).catch(() => {
            store.getState().showNotice(t('connection.saveFailed'))
          })
        }}
      >
        {LANGUAGE_CHOICES.map((c) => (
          <option key={c.value} value={c.value} lang={c.lang}>
            {t(c.text)}
          </option>
        ))}
      </select>
    </div>
  )
}
