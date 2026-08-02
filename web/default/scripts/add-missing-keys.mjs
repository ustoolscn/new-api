import fs from 'node:fs/promises'
import path from 'node:path'
const LOCALES_DIR = path.resolve('src/i18n/locales')
function stableStringify(obj) { return JSON.stringify(obj, null, 2) + '\n' }
const newKeys = {
  en: {
    'Selected period': 'Selected period',
    'Filter invited users by date range. Top-ups, credited balance, commission, and consumption all use the same selected period.':
      'Filter invited users by date range. Top-ups, credited balance, commission, and consumption all use the same selected period.',
  },
  zh: {
    'Selected period': '已选时间范围',
    'Filter invited users by date range. Top-ups, credited balance, commission, and consumption all use the same selected period.':
      '按时间范围筛选受邀用户。充值次数、到账额度、返佣和消耗都使用同一选定区间。',
  },
  fr: {
    'Selected period': 'Période sélectionnée',
    'Filter invited users by date range. Top-ups, credited balance, commission, and consumption all use the same selected period.':
      'Filtrez les utilisateurs invités par période. Recharges, solde crédité, commission et consommation utilisent la même période sélectionnée.',
  },
  ja: {
    'Selected period': '選択した期間',
    'Filter invited users by date range. Top-ups, credited balance, commission, and consumption all use the same selected period.':
      '期間で招待ユーザーを絞り込みます。チャージ回数、入金額、コミッション、消費はすべて同じ期間を使います。',
  },
  ru: {
    'Selected period': 'Выбранный период',
    'Filter invited users by date range. Top-ups, credited balance, commission, and consumption all use the same selected period.':
      'Фильтруйте приглашённых пользователей по периоду. Пополнения, зачисленный баланс, комиссия и расход используют один и тот же выбранный период.',
  },
  vi: {
    'Selected period': 'Kỳ đã chọn',
    'Filter invited users by date range. Top-ups, credited balance, commission, and consumption all use the same selected period.':
      'Lọc người dùng được mời theo khoảng thời gian. Lượt nạp, số dư ghi có, hoa hồng và tiêu thụ đều dùng cùng kỳ đã chọn.',
  },
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
