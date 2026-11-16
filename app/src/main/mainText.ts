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
  confFilter: string
  vpsKeyFilter: string
  notifyErrorTitle: string
  notifyErrorBody: string
  notifyVpnDownTitle: string
  notifyVpnDownBody: string
  notifyBlockedTitle: string
  notifyBlockedBody: string
  notifyLoginTitle: string
  notifyLoginBody: string
  /** {v} is the version. */
  notifyUpdateTitle: string
  notifyUpdateBody: string
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
    zipFilter: 'Архив ZIP',
    confFilter: 'Файлы настроек WireGuard',
    vpsKeyFilter: 'Ключ SSH',
    notifyErrorTitle: 'SplitLane: деление остановлено',
    notifyErrorBody: 'Откройте SplitLane, чтобы узнать причину',
    notifyVpnDownTitle: 'SplitLane: нет связи с VPN-сервером',
    notifyVpnDownBody: 'Программы дорожки VPN без интернета, пока связь не вернётся',
    notifyBlockedTitle: 'SplitLane: Tailscale снова включил exit node',
    notifyBlockedBody: 'Деление на паузе. Откройте SplitLane, чтобы выбрать, что делать',
    notifyLoginTitle: 'SplitLane: войдите в Tailscale снова',
    notifyLoginBody: 'Откройте SplitLane, чтобы войти',
    notifyUpdateTitle: 'SplitLane: доступна версия {v}',
    notifyUpdateBody: 'Нажмите, чтобы открыть SplitLane и обновить'
  },
  en: {
    open: 'Open',
    settings: 'Settings…',
    checkUpdates: 'Check for updates',
    enable: 'Turn on',
    disable: 'Turn off',
    quit: 'Quit',
    programsFilter: 'Programs',
    zipFilter: 'ZIP archive',
    confFilter: 'WireGuard configuration files',
    vpsKeyFilter: 'SSH key',
    notifyErrorTitle: 'SplitLane: split routing stopped',
    notifyErrorBody: 'Open SplitLane to see why',
    notifyVpnDownTitle: "SplitLane: can't reach the VPN server",
    notifyVpnDownBody: 'Programs in the VPN lane have no internet until the connection is back',
    notifyBlockedTitle: 'SplitLane: Tailscale turned its exit node back on',
    notifyBlockedBody: 'Split routing is paused. Open SplitLane to choose what to do',
    notifyLoginTitle: 'SplitLane: log in to Tailscale again',
    notifyLoginBody: 'Open SplitLane to log in',
    notifyUpdateTitle: 'SplitLane: version {v} is available',
    notifyUpdateBody: 'Click to open SplitLane and update'
  }
}

export function mainText(lang: Lang): MainText {
  return MAIN_TEXT[lang]
}
