import type { Lang } from '../shared/i18n'

/** The main process's own strings (the page's dictionary lives in the renderer). */
export interface MainText {
  open: string
  settings: string
  checkUpdates: string
  enable: string
  disable: string
  quit: string
  programsFilter: string
  zipFilter: string
}

export const MAIN_TEXT: Record<Lang, MainText> = {
  ru: {
    open: 'Открыть',
    settings: 'Настройки…',
    checkUpdates: 'Проверить обновления',
    enable: 'Включить',
    disable: 'Выключить',
    quit: 'Выход',
    programsFilter: 'Программы',
    zipFilter: 'Архив ZIP'
  },
  en: {
    open: 'Open',
    settings: 'Settings…',
    checkUpdates: 'Check for updates',
    enable: 'Turn on',
    disable: 'Turn off',
    quit: 'Quit',
    programsFilter: 'Programs',
    zipFilter: 'ZIP archive'
  }
}

export function mainText(lang: Lang): MainText {
  return MAIN_TEXT[lang]
}
