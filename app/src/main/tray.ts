import { Menu, Tray, nativeImage, screen, type NativeImage } from 'electron'
import type { HelperState } from '../shared/protocol'
import type { LinkStatus } from '../shared/status'
import { laneGlyphBitmap, trayIconFor, trayMenuModel, type TrayIcon } from './trayModel'

/** The main process's own strings (the page's dictionary lives in the renderer). */
export const MAIN_TEXT = {
  open: 'Открыть',
  settings: 'Настройки…',
  checkUpdates: 'Проверить обновления',
  enable: 'Включить',
  disable: 'Выключить',
  quit: 'Выход',
  programsFilter: 'Программы',
  zipFilter: 'Архив ZIP'
} as const

const COLORS: Record<TrayIcon, [number, number, number]> = {
  working: [0x8b, 0x7c, 0xff],
  off: [0x8b, 0x93, 0xa7],
  action: [0xff, 0xb4, 0x54],
  'vpn-down': [0xff, 0x6b, 0x6b]
}

/** createFromBitmap takes premultiplied BGRA; laneGlyphBitmap gives straight alpha. */
function premultiplied(bgra: Buffer): Buffer {
  const out = Buffer.from(bgra)
  for (let i = 0; i < out.length; i += 4) {
    const a = out[i + 3]
    out[i] = Math.round((out[i] * a) / 255)
    out[i + 1] = Math.round((out[i + 1] * a) / 255)
    out[i + 2] = Math.round((out[i + 2] * a) / 255)
  }
  return out
}

/**
 * The glyph as an image of `logical` pixels at the scale of the primary display. On Windows Electron makes the icon
 * of a tray or a window from the image's 1x bitmap alone (other representations are not used), so that bitmap is
 * drawn at the size the shell shows: 16 px for the tray and 32 px for the window at 100 %, more on a scaled display.
 */
function glyphImage(rgb: [number, number, number], logical: number): NativeImage {
  const size = Math.max(logical, Math.round(logical * screen.getPrimaryDisplay().scaleFactor))
  return nativeImage.createFromBitmap(premultiplied(laneGlyphBitmap(size, rgb)), {
    width: size,
    height: size,
    scaleFactor: 1
  })
}

/** The icon of the window and its taskbar button: the glyph in the VPN colour. */
export function windowIcon(): NativeImage {
  return glyphImage(COLORS.working, 32)
}

export interface TrayActions {
  open(): void
  /** Shows the window on «Подключение» with the settings. */
  settings(): void
  /** Starts a manual update check and shows the window on «Подключение». */
  checkUpdates(): void
  toggle(to: 'enable' | 'disable'): void
  quit(): void
}

/** The tray icon and its menu (spec 9.6): Открыть, Настройки…, Включить or Выключить, Проверить обновления, Выход. */
export class AppTray {
  private readonly tray: Tray
  private readonly images = new Map<TrayIcon, NativeImage>()
  private icon: TrayIcon = 'off'
  private toggle: 'enable' | 'disable' | null | undefined = undefined

  constructor(private readonly actions: TrayActions) {
    this.tray = new Tray(this.image(this.icon))
    this.tray.setToolTip('SplitLane')
    this.tray.on('click', () => actions.open())
    this.update({ kind: 'connecting' }, null)
  }

  /** Called on every change of the link status or the state. */
  update(link: LinkStatus, state: HelperState | null): void {
    const icon = trayIconFor(link, state, this.icon)
    if (icon !== this.icon) {
      this.icon = icon
      this.tray.setImage(this.image(icon))
    }
    const { toggle } = trayMenuModel(link, state)
    if (toggle === this.toggle) return
    this.toggle = toggle
    this.tray.setContextMenu(
      Menu.buildFromTemplate([
        { label: MAIN_TEXT.open, click: () => this.actions.open() },
        { label: MAIN_TEXT.settings, click: () => this.actions.settings() },
        {
          label: toggle === 'disable' ? MAIN_TEXT.disable : MAIN_TEXT.enable,
          enabled: toggle !== null,
          click: () => {
            if (toggle !== null) this.actions.toggle(toggle)
          }
        },
        { label: MAIN_TEXT.checkUpdates, click: () => this.actions.checkUpdates() },
        { type: 'separator' },
        { label: MAIN_TEXT.quit, click: () => this.actions.quit() }
      ])
    )
  }

  destroy(): void {
    this.tray.destroy()
  }

  private image(icon: TrayIcon): NativeImage {
    let img = this.images.get(icon)
    if (img === undefined) {
      img = glyphImage(COLORS[icon], 16)
      this.images.set(icon, img)
    }
    return img
  }
}
