import fs from 'node:fs/promises'
import path from 'node:path'

const LOCALES_DIR = path.resolve('src/i18n/locales')
function stableStringify(obj) {
  return JSON.stringify(obj, null, 2) + '\n'
}

const newKeys = {
  en: {
    'Last Month': 'Last Month',
    'Last Year': 'Last Year',
    'Period total consumption': 'Period total consumption',
    'Period consumption': 'Period consumption',
    'Lifetime consumption': 'Lifetime consumption',
    'Lifetime invitee consumption': 'Lifetime invitee consumption',
    'Failed to load invitee consumption': 'Failed to load invitee consumption',
    'Please select a valid date range': 'Please select a valid date range',
    'Query each invited user’s consumption for a selected period, plus the period total. Period data comes from durable usage aggregates, not request logs.':
      'Query each invited user’s consumption for a selected period, plus the period total. Period data comes from durable usage aggregates, not request logs.',
  },
  zh: {
    'Last Month': '上月',
    'Last Year': '近一年',
    'Period total consumption': '区间汇总消耗',
    'Period consumption': '区间消耗',
    'Lifetime consumption': '累计消耗',
    'Lifetime invitee consumption': '被邀请人累计消耗',
    'Failed to load invitee consumption': '加载被邀请人消耗失败',
    'Please select a valid date range': '请选择有效的时间范围',
    'Query each invited user’s consumption for a selected period, plus the period total. Period data comes from durable usage aggregates, not request logs.':
      '可按时间范围查询每位被邀请用户的消耗，并显示区间汇总。区间数据来自持久化用量汇总，不依赖请求日志。',
  },
  fr: {
    'Last Month': 'Mois dernier',
    'Last Year': 'Dernière année',
    'Period total consumption': 'Consommation totale de la période',
    'Period consumption': 'Consommation de la période',
    'Lifetime consumption': 'Consommation totale',
    'Lifetime invitee consumption': 'Consommation cumulée des invités',
    'Failed to load invitee consumption': 'Échec du chargement de la consommation des invités',
    'Please select a valid date range': 'Veuillez sélectionner une plage de dates valide',
    'Query each invited user’s consumption for a selected period, plus the period total. Period data comes from durable usage aggregates, not request logs.':
      'Consultez la consommation de chaque utilisateur invité pour une période sélectionnée, ainsi que le total de la période. Les données de période proviennent d’agrégats d’usage durables, pas des journaux de requêtes.',
  },
  ja: {
    'Last Month': '先月',
    'Last Year': '過去1年',
    'Period total consumption': '期間合計消費',
    'Period consumption': '期間消費',
    'Lifetime consumption': '累計消費',
    'Lifetime invitee consumption': '被招待ユーザー累計消費',
    'Failed to load invitee consumption': '被招待ユーザーの消費を読み込めませんでした',
    'Please select a valid date range': '有効な期間を選択してください',
    'Query each invited user’s consumption for a selected period, plus the period total. Period data comes from durable usage aggregates, not request logs.':
      '選択した期間の各招待ユーザー消費と期間合計を表示します。期間データはリクエストログではなく、永続化された利用量集計に基づきます。',
  },
  ru: {
    'Last Month': 'Прошлый месяц',
    'Last Year': 'Последний год',
    'Period total consumption': 'Суммарный расход за период',
    'Period consumption': 'Расход за период',
    'Lifetime consumption': 'Суммарный расход',
    'Lifetime invitee consumption': 'Суммарный расход приглашённых',
    'Failed to load invitee consumption': 'Не удалось загрузить расход приглашённых',
    'Please select a valid date range': 'Выберите корректный диапазон дат',
    'Query each invited user’s consumption for a selected period, plus the period total. Period data comes from durable usage aggregates, not request logs.':
      'Показывает расход каждого приглашённого пользователя за выбранный период и итог периода. Данные периода берутся из устойчивых агрегатов использования, а не из логов запросов.',
  },
  vi: {
    'Last Month': 'Tháng trước',
    'Last Year': 'Một năm qua',
    'Period total consumption': 'Tổng tiêu thụ theo kỳ',
    'Period consumption': 'Tiêu thụ theo kỳ',
    'Lifetime consumption': 'Tiêu thụ tích lũy',
    'Lifetime invitee consumption': 'Tổng tiêu thụ người được mời',
    'Failed to load invitee consumption': 'Không tải được tiêu thụ người được mời',
    'Please select a valid date range': 'Vui lòng chọn khoảng thời gian hợp lệ',
    'Query each invited user’s consumption for a selected period, plus the period total. Period data comes from durable usage aggregates, not request logs.':
      'Tra cứu mức tiêu thụ của từng người dùng được mời theo kỳ đã chọn và tổng kỳ. Dữ liệu theo kỳ lấy từ tổng hợp sử dụng bền vững, không phụ thuộc nhật ký yêu cầu.',
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
    } else if (locale !== 'en' && json.translation[key] === key) {
      json.translation[key] = value
      count++
    }
  }
  json.translation = Object.fromEntries(Object.entries(json.translation).sort(([a],[b]) => a.localeCompare(b)))
  await fs.writeFile(filePath, stableStringify(json), 'utf8')
  console.log(locale, count)
}
