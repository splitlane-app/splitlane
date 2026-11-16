/** A neutral tile with a letter, for a program whose real icon is not at hand (never a vendor logo). */
export function monogram(name: string): string {
  const letter = (name.trim()[0] ?? '?').toUpperCase()
  const svg =
    '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64">' +
    '<rect width="64" height="64" rx="14" fill="#3a4050"/>' +
    '<text x="32" y="33" text-anchor="middle" dominant-baseline="central" ' +
    'font-family="Segoe UI, Arial, sans-serif" font-size="34" font-weight="600" fill="#e6e8ee">' +
    letter +
    '</text></svg>'
  return `data:image/svg+xml;base64,${btoa(svg)}`
}
