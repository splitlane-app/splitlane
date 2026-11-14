import { Menu, Tray, nativeImage, type NativeImage } from 'electron'
import type { HelperState } from '../shared/protocol'
import type { LinkStatus } from '../shared/status'
import { circleBitmap, trayIconFor, trayMenuModel, type TrayIcon } from './trayModel'

/** The main process's own strings (the page's dictionary lives in the renderer). */
export const MAIN_TEXT = {
  open: 'Открыть',
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

/** createFromBitmap and addRepresentation take premultiplied BGRA; circleBitmap gives straight alpha. */
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

function trayImage(icon: TrayIcon): NativeImage {
  const rgb = COLORS[icon]
  const img = nativeImage.createFromBitmap(premultiplied(circleBitmap(16, rgb)), { width: 16, height: 16, scaleFactor: 1 })
  img.addRepresentation({ scaleFactor: 2, width: 32, height: 32, buffer: premultiplied(circleBitmap(32, rgb)) })
  return img
}

export interface TrayActions {
  open(): void
  toggle(to: 'enable' | 'disable'): void
  quit(): void
}

/** The tray icon and its menu (spec 9.6): Открыть, Включить or Выключить, Выход. */
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
        {
          label: toggle === 'disable' ? MAIN_TEXT.disable : MAIN_TEXT.enable,
          enabled: toggle !== null,
          click: () => {
            if (toggle !== null) this.actions.toggle(toggle)
          }
        },
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
      img = trayImage(icon)
      this.images.set(icon, img)
    }
    return img
  }
}
