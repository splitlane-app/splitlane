import { t } from '../i18n/t'

const KB = 1024
const MB = 1024 * 1024

function oneDecimal(n: number): string {
  return n.toFixed(1).replace('.', ',')
}

export function formatSpeed(bytesPerSecond: number): string {
  if (!(bytesPerSecond > 0)) return ''
  if (bytesPerSecond < KB) return `${Math.round(bytesPerSecond)} ${t('unit.bps')}`
  if (bytesPerSecond < MB) return `${oneDecimal(bytesPerSecond / KB)} ${t('unit.kbps')}`
  return `${oneDecimal(bytesPerSecond / MB)} ${t('unit.mbps')}`
}

export function maskAddress(ip: string | null): string {
  if (ip === null) return ''
  const v4 = /^(\d{1,3})\.(\d{1,3})\.\d{1,3}\.\d{1,3}$/.exec(ip)
  if (v4) return `${v4[1]}.${v4[2]}.•.•`
  return `${ip.split(':')[0]}:•`
}
