/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  Bot,
  ChevronRight,
  Clock,
  ClipboardCheck,
  CloudCog,
  Download,
  FileKey2,
  Gauge,
  Languages,
  LockKeyhole,
  MonitorCog,
  PackageCheck,
  PlugZap,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Sparkles,
  Wrench,
  type LucideIcon,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { PublicLayout } from '@/components/layout'
import { useTheme } from '@/context/theme-provider'

import { getHiCodexResolvedTheme, type HiCodexResolvedTheme } from './lib/theme'

import './hi-codex.css'

const HI_CODEX_ASSET_BASE = '/assets/hi-codex'

type HiCodexUpdate = {
  notes: string
  sha256: string
  url: string
  version: string
}

type LatestUpdateStatus = 'error' | 'loading' | 'ready'

type DownloadState = 'error' | 'idle' | 'working'

type IconContent = {
  icon: LucideIcon
  text: string
  title: string
}

type PlatformDownloadsProps = {
  compact?: boolean
  error: string
  load: () => Promise<HiCodexUpdate>
  status: LatestUpdateStatus
  update: HiCodexUpdate | null
}

const UPDATE_ENDPOINT = '/hi-codex/update.json'

const heroStats = [
  { value: '一键', label: '配置 API Key 与 provider' },
  { value: '200+', label: '插件汉化与可用性修复' },
  { value: '模型', label: '同步 Responses 模型列表' },
]

const heroHighlights = [
  '一键配置 API Key',
  'Codex 中文界面',
  '200+ 插件汉化可用',
  '模型列表同步',
]

const featureCards: IconContent[] = [
  {
    icon: FileKey2,
    title: 'Cooper 账号与 API Key',
    text: '登录、选择 Key、写入 auth.json/config.toml，并展示余额与套餐用量。',
  },
  {
    icon: MonitorCog,
    title: 'Codex 启停与 DevTools',
    text: '自动检测 Codex.exe，重启并打开调试端口，恢复页面注入能力。',
  },
  {
    icon: Languages,
    title: '模型同步与中文界面',
    text: '拉取 Responses 模型列表，注入模型选择器，并写入本地语言缓存。',
  },
  {
    icon: PlugZap,
    title: '插件市场与修复',
    text: '添加 CooperAPI-Plugin 市场，补齐 Git、runtime 与常见 package 配置。',
  },
  {
    icon: Smartphone,
    title: '手机消息接入 Codex',
    text: '把 QQ、微信、飞书消息投递到绑定会话，并同步 Codex 回复。',
  },
  {
    icon: ShieldCheck,
    title: '更新与本地安全',
    text: '自动获取最新版安装包、校验 SHA256，并用 DPAPI 保存敏感信息。',
  },
]

const workflow: IconContent[] = [
  {
    icon: LockKeyhole,
    title: '登录 Cooper',
    text: '读取可用 Key、余额与套餐用量。',
  },
  {
    icon: CloudCog,
    title: '配置 Codex',
    text: '自动写入 provider、Key 与本地配置。',
  },
  {
    icon: Wrench,
    title: '修复环境',
    text: '模型、汉化、插件、历史和国内镜像集中处理。',
  },
  { icon: Bot, title: '接入消息', text: '将手机端消息绑定到指定 Codex 会话。' },
]

const usageSteps = [
  {
    title: '下载并安装 Hi Codex',
    text: '点击页面首屏或下载区的 Windows 下载按钮，官网会自动获取最新版安装包地址并开始下载。安装完成后从开始菜单或桌面快捷方式启动。',
  },
  {
    title: '确认 Codex App 与 Codex Home',
    text: '首次打开后进入高级设置，检查 Codex.exe 是否自动识别，Codex home 通常指向用户目录下的 .codex。路径不对时可以手动选择。',
  },
  {
    title: '登录 Cooper 并写入 API Key',
    text: '在账号区域登录 Cooper，读取可用 API Key 后点击配置 APIKEY。多 Key 用户可以在弹窗里选择要写入 Codex 的 Key。',
  },
  {
    title: '重启 Codex 并打开 DevTools 端口',
    text: '点击重启 Codex，Hi Codex 会用指定 DevTools 端口启动 Codex App，后续模型同步、汉化按钮和页面增强都依赖这个连接。',
  },
  {
    title: '同步模型、汉化和插件',
    text: '进入工具箱，按需执行同步模型列表、一键汉化、添加插件市场、修复插件、修复 Codex 和国内优化。遇到异常时先打开诊断查看原因。',
  },
  {
    title: '绑定 QQ / 微信 / 飞书会话',
    text: '在连接手机区域选择平台，扫码或填写机器人配置，刷新当前 Codex 可见会话并绑定。绑定后手机消息会投递到指定 Codex 会话，回复也会同步回手机端。',
  },
]

const usageFaq = [
  {
    title: '配置 APIKEY 会覆盖我的文件吗？',
    text: 'Hi Codex 会写入 Codex 所需的 auth.json 和 config.toml provider 配置，目标是减少手动改配置。建议首次使用前确认 Codex home 路径正确。',
  },
  {
    title: '汉化会修改 Codex App 主程序吗？',
    text: '不会替换 app.asar。汉化通过写入本地语言缓存和刷新页面实现，必要时可以重新诊断或恢复 Codex 页面状态。',
  },
  {
    title: '插件修复主要修什么？',
    text: '主要处理插件市场配置、primary runtime、node_modules 目录、package exports 等 Windows 上常见运行时问题。',
  },
  {
    title: '模型同步失败怎么办？',
    text: '先确认 Cooper provider 和 API Key 已配置，再打开工具箱里的模型诊断。DevTools 端口未连接时，先重启 Codex。',
  },
]

const screenshots = [
  {
    src: `${HI_CODEX_ASSET_BASE}/screenshots/03-tools-drawer.png`,
    title: '工具箱',
    text: '汉化、模型同步、历史同步、插件市场和修复入口集中在一个抽屉里。',
  },
  {
    src: `${HI_CODEX_ASSET_BASE}/screenshots/07-phone-qq-drawer.png`,
    title: '手机连接',
    text: 'QQ、微信、飞书可绑定会话，将外部消息同步到 Codex。',
  },
  {
    src: `${HI_CODEX_ASSET_BASE}/screenshots/10-api-key-modal.png`,
    title: 'Key 选择',
    text: '从 Cooper 账号中选择要写入 Codex 的 API Key。',
  },
]

const seoKeywords = ['Codex', '汉化', '插件', '模型']

function normalizeUpdate(payload: unknown): HiCodexUpdate {
  if (!payload || typeof payload !== 'object') {
    throw new Error('暂时无法获取最新版')
  }

  const record = payload as Record<string, unknown>
  const version = String(record.version || '').trim()
  const url = String(record.url || '').trim()
  const notes = String(record.notes || '').trim()
  const sha256 = String(record.sha256 || '').trim()

  if (!version || !url) {
    throw new Error('暂时无法获取下载地址')
  }

  return { version, url, notes, sha256 }
}

async function fetchLatestUpdate(): Promise<HiCodexUpdate> {
  try {
    const response = await fetch(`${UPDATE_ENDPOINT}?t=${Date.now()}`, {
      cache: 'no-store',
    })

    if (!response.ok) {
      throw new Error(`${UPDATE_ENDPOINT} 返回 ${response.status}`)
    }

    return normalizeUpdate(await response.json())
  } catch {
    throw new Error('暂时无法获取最新版')
  }
}

function useLatestUpdate() {
  const [update, setUpdate] = useState<HiCodexUpdate | null>(null)
  const [status, setStatus] = useState<LatestUpdateStatus>('loading')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setStatus('loading')
    setError('')
    try {
      const nextUpdate = await fetchLatestUpdate()
      setUpdate(nextUpdate)
      setStatus('ready')
      return nextUpdate
    } catch (err) {
      setStatus('error')
      setError(err instanceof Error ? err.message : '无法读取更新信息')
      throw err
    }
  }, [])

  useEffect(() => {
    load().catch(() => {})
  }, [load])

  return { update, status, error, load }
}

function PlatformDownloads(props: PlatformDownloadsProps) {
  const [downloadState, setDownloadState] = useState<DownloadState>('idle')

  const handleDownload = async () => {
    setDownloadState('working')
    try {
      const latest = props.update || (await props.load())
      window.location.assign(latest.url)
      setDownloadState('idle')
    } catch {
      setDownloadState('error')
    }
  }

  return (
    <div
      className={
        props.compact ? 'platform-downloads compact' : 'platform-downloads'
      }
    >
      <button
        className='download-button'
        type='button'
        onClick={handleDownload}
      >
        {downloadState === 'working' ? (
          <RefreshCw size={20} className='spin' />
        ) : (
          <Download size={20} />
        )}
        {downloadState === 'working' ? '正在获取下载地址' : '下载 Windows 版'}
      </button>
      <button
        className='mac-button'
        type='button'
        disabled
        aria-disabled='true'
      >
        <Clock size={20} />
        Mac 版暂待更新
      </button>
      {props.compact && props.status === 'ready' ? (
        <span className='hero-version'>最新版 {props.update?.version}</span>
      ) : null}
      {props.compact && props.status === 'loading' ? (
        <span className='hero-version'>正在获取最新版</span>
      ) : null}
      {props.compact &&
      (props.status === 'error' || downloadState === 'error') ? (
        <span className='download-inline-error'>
          {props.error || '暂时无法获取下载地址'}
        </span>
      ) : null}
      {!props.compact && downloadState === 'error' ? (
        <p className='error-text'>请稍后重试，当前未能获取下载地址。</p>
      ) : null}
    </div>
  )
}

function GuidePanel() {
  const [activeStep, setActiveStep] = useState(0)
  const current = usageSteps[activeStep]
  return (
    <section className='section guide-section' id='guide'>
      <div className='section-heading'>
        <p className='section-kicker'>Guide</p>
        <h2>Hi Codex 使用说明</h2>
        <p>
          按真实上手路径组织：下载、配置 API
          Key、同步模型、汉化插件、绑定手机会话。
        </p>
      </div>
      <div className='guide-stage'>
        <div className='step-selector' role='tablist' aria-label='使用步骤'>
          {usageSteps.map((step, index) => (
            <button
              aria-selected={activeStep === index}
              className={activeStep === index ? 'active' : ''}
              key={step.title}
              onClick={() => setActiveStep(index)}
              role='tab'
              type='button'
            >
              <span>{String(index + 1).padStart(2, '0')}</span>
              {step.title}
            </button>
          ))}
        </div>
        <article className='guide-detail' role='tabpanel'>
          <span>{String(activeStep + 1).padStart(2, '0')}</span>
          <h3>{current.title}</h3>
          <p>{current.text}</p>
          <div className='guide-progress' aria-hidden='true'>
            <i
              style={{
                width: `${((activeStep + 1) / usageSteps.length) * 100}%`,
              }}
            />
          </div>
        </article>
        <aside className='usage-faq' aria-label='使用常见问题'>
          <h3>常见问题</h3>
          {usageFaq.map((item) => (
            <details key={item.title}>
              <summary>{item.title}</summary>
              <p>{item.text}</p>
            </details>
          ))}
        </aside>
      </div>
    </section>
  )
}

export function HiCodexPage() {
  const updateState = useLatestUpdate()
  const { update, status, error, load } = updateState
  const { resolvedTheme, theme } = useTheme()
  const pageTheme: HiCodexResolvedTheme = getHiCodexResolvedTheme(
    theme,
    resolvedTheme
  )

  return (
    <PublicLayout showMainContainer={false}>
      <main className='hi-codex-page' data-theme={pageTheme}>
        <div className='scene-fx' aria-hidden='true'>
          <span className='grid-plane' />
          <span className='scanline' />
        </div>

        <section className='hero' id='top'>
          <div className='hero-copy'>
            <p className='eyebrow'>
              <Sparkles size={16} />
              Codex App 的桌面增强面板
            </p>
            <h1>Hi Codex</h1>
            <p className='hero-subtitle'>
              一键配置 API Key、开启 Codex 汉化、同步模型列表，并让 200+
              插件汉化后可用。QQ、微信、飞书消息也能接入 Codex 会话。
            </p>
            <div className='hero-actions'>
              <PlatformDownloads
                update={update}
                status={status}
                error={error}
                load={load}
                compact
              />
              <a className='secondary-link' href='#features'>
                看核心能力
                <ChevronRight size={18} />
              </a>
            </div>
            <div className='hero-highlights' aria-label='首屏核心功能'>
              {heroHighlights.map((item) => (
                <span key={item}>{item}</span>
              ))}
            </div>
            <div className='hero-stats' aria-label='产品摘要'>
              {heroStats.map((stat) => (
                <div key={stat.label}>
                  <strong>{stat.value}</strong>
                  <span>{stat.label}</span>
                </div>
              ))}
            </div>
            <div className='command-rail' aria-label='核心能力动效展示'>
              <span>cooper login</span>
              <span>sync models</span>
              <span>patch plugins</span>
              <span>bind mobile</span>
            </div>
          </div>
          <div className='hero-visual' aria-label='Hi Codex 主界面截图'>
            <img
              className='app-shot ghost-shot ghost-one'
              src={`${HI_CODEX_ASSET_BASE}/screenshots/03-tools-drawer.png`}
              alt=''
            />
            <img
              className='app-shot ghost-shot ghost-two'
              src={`${HI_CODEX_ASSET_BASE}/screenshots/10-api-key-modal.png`}
              alt=''
            />
            <img
              className='app-shot main-shot'
              src={`${HI_CODEX_ASSET_BASE}/screenshots/01-main-dashboard.png`}
              alt='Hi Codex 主控制面板'
            />
            <img
              className='app-icon-large'
              src={`${HI_CODEX_ASSET_BASE}/brand/app_icon.png`}
              alt=''
            />
          </div>
        </section>

        <section className='trust-band' aria-label='安全与交付特性'>
          <div>
            <ClipboardCheck size={22} />
            <span>不修改 Codex App 主程序</span>
          </div>
          <div>
            <Gauge size={22} />
            <span>余额、套餐和 token 用量可视化</span>
          </div>
          <div>
            <PackageCheck size={22} />
            <span>安装包 SHA256 校验</span>
          </div>
        </section>

        <section className='seo-strip' aria-label='Hi Codex 关键词能力'>
          <p>围绕 Codex App 的本地增强能力</p>
          <div>
            {seoKeywords.map((keyword) => (
              <span key={keyword}>{keyword}</span>
            ))}
          </div>
        </section>

        <section className='section' id='features'>
          <div className='section-heading'>
            <p className='section-kicker'>Features</p>
            <h2>Codex 汉化、插件和模型同步都收进一个小窗口</h2>
            <p>
              Hi Codex 面向国内网络和多平台消息场景，把 Codex
              账号配置、模型同步、插件市场、汉化、历史修复和手机同步做成可操作的桌面控制面板。
            </p>
          </div>
          <div className='feature-grid'>
            {featureCards.map((feature) => {
              const Icon = feature.icon
              return (
                <article className='feature-card' key={feature.title}>
                  <Icon size={24} />
                  <h3>{feature.title}</h3>
                  <p>{feature.text}</p>
                </article>
              )
            })}
          </div>
        </section>

        <section className='workflow'>
          <div className='workflow-copy'>
            <p className='section-kicker'>Workflow</p>
            <h2>从安装到接入 Codex，会走得很短</h2>
            <p>
              适合已经在桌面端使用 Codex
              App、但希望减少手动配置和环境排查成本的用户。
            </p>
          </div>
          <div className='workflow-steps'>
            {workflow.map((step, index) => {
              const Icon = step.icon
              return (
                <article key={step.title}>
                  <span>{String(index + 1).padStart(2, '0')}</span>
                  <Icon size={23} />
                  <h3>{step.title}</h3>
                  <p>{step.text}</p>
                </article>
              )
            })}
          </div>
        </section>

        <GuidePanel />

        <section className='section screen-section' id='screens'>
          <div className='section-heading'>
            <p className='section-kicker'>Screens</p>
            <h2>真实 WebView UI 截图</h2>
            <p>截图来自演示数据，不包含真实账号、Key 或本机敏感路径。</p>
          </div>
          <div className='screen-layout'>
            <div className='showcase-device'>
              <img
                className='phone-shot'
                src={`${HI_CODEX_ASSET_BASE}/screenshots/01-main-dashboard.png`}
                alt='Hi Codex 主控制面板完整截图'
              />
            </div>
            <div className='screen-cards'>
              {screenshots.map((screen) => (
                <article key={screen.title}>
                  <img src={screen.src} alt={`Hi Codex ${screen.title}截图`} />
                  <div>
                    <h3>{screen.title}</h3>
                    <p>{screen.text}</p>
                  </div>
                </article>
              ))}
            </div>
          </div>
        </section>

        <footer>
          <div className='footer-brand'>
            <img src={`${HI_CODEX_ASSET_BASE}/brand/app_icon_ui.png`} alt='' />
            <span>Hi Codex</span>
          </div>
          <p>Desktop helper and enhancement panel for Codex App.</p>
        </footer>
      </main>
    </PublicLayout>
  )
}
