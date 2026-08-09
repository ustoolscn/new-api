import fs from 'node:fs/promises'
import path from 'node:path'
const LOCALES_DIR = path.resolve('src/i18n/locales')
function stableStringify(obj) {
  return JSON.stringify(obj, null, 2) + '\n'
}
const newKeys = {
  en: {
    'Add invitee': 'Add invitee',
    'Adding...': 'Adding...',
    'Invitee added successfully': 'Invitee added successfully',
    'Failed to add invitee': 'Failed to add invitee',
    'Please enter a user ID or username': 'Please enter a user ID or username',
    'User ID or username': 'User ID or username',
    'Enter user ID or username': 'Enter user ID or username',
    'Link an existing user as an invitee of {{username}}. This only sets the invite relationship and does not grant registration rewards.':
      'Link an existing user as an invitee of {{username}}. This only sets the invite relationship and does not grant registration rewards.',
    'Link an existing user as an invitee. This only sets the invite relationship and does not grant registration rewards.':
      'Link an existing user as an invitee. This only sets the invite relationship and does not grant registration rewards.',
    'Total invitation revenue including registration rewards and top-up commissions':
      'Total invitation revenue including registration rewards and top-up commissions',
    'Invitation Code must be 32 characters or fewer':
      'Invitation Code must be 32 characters or fewer',
  },
  zh: {
    'Add invitee': '添加被邀请人',
    'Adding...': '添加中...',
    'Invitee added successfully': '被邀请人添加成功',
    'Failed to add invitee': '添加被邀请人失败',
    'Please enter a user ID or username': '请输入用户 ID 或用户名',
    'User ID or username': '用户 ID 或用户名',
    'Enter user ID or username': '输入用户 ID 或用户名',
    'Link an existing user as an invitee of {{username}}. This only sets the invite relationship and does not grant registration rewards.':
      '将已有用户绑定为 {{username}} 的被邀请人。仅设置邀请关系，不会发放注册邀请奖励。',
    'Link an existing user as an invitee. This only sets the invite relationship and does not grant registration rewards.':
      '将已有用户绑定为被邀请人。仅设置邀请关系，不会发放注册邀请奖励。',
    'Total invitation revenue including registration rewards and top-up commissions':
      '邀请总收入（含注册邀请奖励与充值返佣）',
    'Invitation Code must be 32 characters or fewer':
      '邀请码长度不能超过 32 个字符',
  },
  fr: {
    'Add invitee': 'Ajouter un invité',
    'Adding...': 'Ajout...',
    'Invitee added successfully': 'Invité ajouté avec succès',
    'Failed to add invitee': 'Échec de l’ajout de l’invité',
    'Please enter a user ID or username':
      'Veuillez saisir un ID utilisateur ou un nom d’utilisateur',
    'User ID or username': 'ID utilisateur ou nom d’utilisateur',
    'Enter user ID or username': 'Saisir l’ID ou le nom d’utilisateur',
    'Link an existing user as an invitee of {{username}}. This only sets the invite relationship and does not grant registration rewards.':
      'Associer un utilisateur existant comme invité de {{username}}. Cela définit uniquement la relation d’invitation et n’accorde pas de récompenses d’inscription.',
    'Link an existing user as an invitee. This only sets the invite relationship and does not grant registration rewards.':
      'Associer un utilisateur existant comme invité. Cela définit uniquement la relation d’invitation et n’accorde pas de récompenses d’inscription.',
    'Total invitation revenue including registration rewards and top-up commissions':
      'Revenu total d’invitation incluant les récompenses d’inscription et les commissions de recharge',
    'Invitation Code must be 32 characters or fewer':
      'Le code d’invitation doit comporter au plus 32 caractères',
  },
  ja: {
    'Add invitee': '被招待ユーザーを追加',
    'Adding...': '追加中...',
    'Invitee added successfully': '被招待ユーザーを追加しました',
    'Failed to add invitee': '被招待ユーザーの追加に失敗しました',
    'Please enter a user ID or username':
      'ユーザーIDまたはユーザー名を入力してください',
    'User ID or username': 'ユーザーIDまたはユーザー名',
    'Enter user ID or username': 'ユーザーIDまたはユーザー名を入力',
    'Link an existing user as an invitee of {{username}}. This only sets the invite relationship and does not grant registration rewards.':
      '既存ユーザーを {{username}} の被招待ユーザーとして紐付けます。招待関係のみ設定し、登録時の招待報酬は付与しません。',
    'Link an existing user as an invitee. This only sets the invite relationship and does not grant registration rewards.':
      '既存ユーザーを被招待ユーザーとして紐付けます。招待関係のみ設定し、登録時の招待報酬は付与しません。',
    'Total invitation revenue including registration rewards and top-up commissions':
      '登録招待報酬とチャージコミッションを含む招待総収益',
    'Invitation Code must be 32 characters or fewer':
      '招待コードは32文字以内で入力してください',
  },
  ru: {
    'Add invitee': 'Добавить приглашённого',
    'Adding...': 'Добавление...',
    'Invitee added successfully': 'Приглашённый пользователь добавлен',
    'Failed to add invitee': 'Не удалось добавить приглашённого',
    'Please enter a user ID or username': 'Введите ID или имя пользователя',
    'User ID or username': 'ID или имя пользователя',
    'Enter user ID or username': 'Введите ID или имя пользователя',
    'Link an existing user as an invitee of {{username}}. This only sets the invite relationship and does not grant registration rewards.':
      'Привязать существующего пользователя как приглашённого для {{username}}. Устанавливается только связь приглашения, регистрационные награды не выдаются.',
    'Link an existing user as an invitee. This only sets the invite relationship and does not grant registration rewards.':
      'Привязать существующего пользователя как приглашённого. Устанавливается только связь приглашения, регистрационные награды не выдаются.',
    'Total invitation revenue including registration rewards and top-up commissions':
      'Суммарный доход от приглашений, включая регистрационные награды и комиссии с пополнений',
    'Invitation Code must be 32 characters or fewer':
      'Код приглашения должен содержать не более 32 символов',
  },
  vi: {
    'Add invitee': 'Thêm người được mời',
    'Adding...': 'Đang thêm...',
    'Invitee added successfully': 'Đã thêm người được mời',
    'Failed to add invitee': 'Thêm người được mời thất bại',
    'Please enter a user ID or username':
      'Vui lòng nhập ID hoặc tên người dùng',
    'User ID or username': 'ID hoặc tên người dùng',
    'Enter user ID or username': 'Nhập ID hoặc tên người dùng',
    'Link an existing user as an invitee of {{username}}. This only sets the invite relationship and does not grant registration rewards.':
      'Liên kết người dùng hiện có thành người được mời của {{username}}. Chỉ thiết lập quan hệ mời, không cấp thưởng đăng ký.',
    'Link an existing user as an invitee. This only sets the invite relationship and does not grant registration rewards.':
      'Liên kết người dùng hiện có thành người được mời. Chỉ thiết lập quan hệ mời, không cấp thưởng đăng ký.',
    'Total invitation revenue including registration rewards and top-up commissions':
      'Tổng thu nhập mời bao gồm thưởng đăng ký và hoa hồng nạp tiền',
    'Invitation Code must be 32 characters or fewer':
      'Mã mời không được dài quá 32 ký tự',
  },
  'zh-TW': {
    'Invitation Code must be 32 characters or fewer':
      '邀請碼長度不能超過 32 個字元',
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
  json.translation = Object.fromEntries(
    Object.entries(json.translation).sort(([a], [b]) => a.localeCompare(b))
  )
  await fs.writeFile(filePath, stableStringify(json), 'utf8')
  console.log(locale, count)
}
