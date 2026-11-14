import { createHash } from 'node:crypto'

export interface IconSource {
  (exe: string): Promise<{ png: Buffer } | null>
}

export interface IconFs {
  stat(p: string): Promise<{ mtimeMs: number } | null>
  read(p: string): Promise<Buffer | null>
  write(p: string, b: Buffer): Promise<void>
}

function dataUrl(png: Buffer): string {
  return `data:image/png;base64,${png.toString('base64')}`
}

/** Program icons cached on disk as <dir>\<sha1>.png, keyed by path and modification time. */
export class IconCache {
  private readonly inflight = new Map<string, Promise<string | null>>()

  constructor(
    private readonly dir: string,
    private readonly source: IconSource,
    private readonly fs: IconFs,
  ) {}

  async get(exe: string): Promise<string | null> {
    const st = await this.fs.stat(exe)
    if (st === null) return null
    const key = createHash('sha1').update(`${exe.toLowerCase()}|${st.mtimeMs}`).digest('hex')
    let p = this.inflight.get(key)
    if (p === undefined) {
      p = this.load(exe, `${this.dir}\\${key}.png`).finally(() => this.inflight.delete(key))
      this.inflight.set(key, p)
    }
    return p
  }

  private async load(exe: string, file: string): Promise<string | null> {
    const cached = await this.fs.read(file)
    if (cached !== null) return dataUrl(cached)
    const icon = await this.source(exe)
    if (icon === null) return null
    try {
      await this.fs.write(file, icon.png)
    } catch {
      // the icon is still returned
    }
    return dataUrl(icon.png)
  }
}
