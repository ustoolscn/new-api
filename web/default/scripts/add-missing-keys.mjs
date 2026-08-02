import fs from 'node:fs/promises'
import path from 'node:path'

const LOCALES_DIR = path.resolve('src/i18n/locales')
function stableStringify(obj) {
  return JSON.stringify(obj, null, 2) + '\n'
}
const newKeys = {
  en: { 'Calendar month': 'Calendar month' },
  zh: { 'Calendar month': '月份' },
  fr: { 'Calendar month': 'Mois' },
  ja: { 'Calendar month': '年月' },
  ru: { 'Calendar month': 'Календарный месяц' },
  vi: { 'Calendar month': 'Tháng' },
}
for (const [locale, trans] of Object.entries(newKeys)) {
  const filePath = path.join(LOCALES_DIR, `${locale}.json`)
  const json = JSON.parse(await fs.readFile(filePath, 'utf8'))
  let count = 0
  for (const [key, value] of Object.entries(trans)) {
    if (!Object.prototype.hasOwnProperty.call(json.translation, key)) {
      json.translation[key] = value
      count++
    }
  }
  json.translation = Object.fromEntries(Object.entries(json.translation).sort(([a],[b]) => a.localeCompare(b)))
  await fs.writeFile(filePath, stableStringify(json), 'utf8')
  console.log(locale, count)
}
