import { useEffect, useState } from 'react'
import { motion } from 'motion/react'
import {
  Activity,
  CheckCircle2,
  Database,
  KeyRound,
  Lock,
  Mail,
  Server,
  ShieldCheck,
  UserRound,
  XCircle,
} from 'lucide-react'
import { PageHeader } from '../components/PageHeader'
import { api, type Stats } from '../api'
import { useAuth } from '../components/AuthProvider'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

const ENV_ROWS: [string, string, string][] = [
  ['GM_ADMIN_USER', 'admin', '管理员用户名'],
  ['GM_ADMIN_PASSWORD', 'admin123', '管理员密码，首次部署务必修改'],
  ['GM_SECRET', '（自动生成）', '凭据加密密钥；修改后旧数据无法解密'],
  ['GM_DATA_DIR', '/data', 'SQLite 与密钥文件目录（需挂载卷持久化）'],
  ['GM_ADDR', ':8080', 'HTTP 监听地址'],
  ['GM_IMAP_HOST', 'imap.gmail.com', 'IMAP 服务器'],
  ['GM_IMAP_PORT', '993', 'IMAP TLS 端口'],
  ['GM_FETCH_CONCURRENCY', '6', '服务端并发取件数'],
  ['GM_FETCH_TIMEOUT_SECONDS', '60', '单账号取件超时'],
  ['GM_FETCH_LIMIT', '10', '单账号默认取件数量上限'],
  ['GM_COOKIE_SECURE', 'false', '启用 HTTPS 后设为 true'],
  ['GM_SESSION_HOURS', '12', '会话有效期（小时）'],
]

export function SettingsPage() {
  const { user } = useAuth()
  const [stats, setStats] = useState<Stats | null>(null)

  useEffect(() => {
    api.stats().then(setStats).catch(() => undefined)
  }, [])

  const cards = [
    { label: '账号总数', value: stats?.total, icon: Mail, tone: 'text-primary' },
    { label: '已配置应用密码', value: stats?.withAppPassword, icon: KeyRound, tone: 'text-sky-600 dark:text-sky-400' },
    { label: '取件正常', value: stats?.fetchOk, icon: CheckCircle2, tone: 'text-emerald-600 dark:text-emerald-400' },
    { label: '取件失败', value: stats?.fetchFailed, icon: XCircle, tone: 'text-destructive' },
    { label: '今日已取件', value: stats?.fetchedToday, icon: Activity, tone: 'text-amber-600 dark:text-amber-400' },
    { label: '当前管理员', value: user?.username ?? '—', icon: UserRound, tone: 'text-foreground' },
  ]

  return (
    <div className="max-w-[860px] space-y-3">
      <PageHeader title="设置" subtitle="部署参数与安全说明（只读，通过环境变量配置）" />

      <Panel title="运行状态" icon={<Activity className="size-3" />}>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          {cards.map((c, i) => {
            const Icon = c.icon
            return (
              <motion.div
                key={c.label}
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: i * 0.04, duration: 0.28 }}
                className="rounded-lg border border-border bg-muted/25 px-3 py-2"
              >
                <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                  <Icon className={cn('size-3', c.tone)} />
                  {c.label}
                </div>
                <p className="mt-0.5 text-[17px] leading-tight font-semibold tabular-nums">
                  {c.value ?? '—'}
                </p>
              </motion.div>
            )
          })}
        </div>
      </Panel>

      <Panel title="环境变量" icon={<Server className="size-3" />}>
        <div>
          {ENV_ROWS.map(([name, def, desc], i) => (
            <div key={name}>
              {i > 0 && <Separator />}
              <div className="flex flex-col gap-1 py-2 sm:flex-row sm:flex-wrap sm:items-baseline sm:gap-x-3 sm:gap-y-1 sm:py-1.5">
                <code className="font-mono text-[11px] break-all text-primary sm:min-w-[210px]">
                  {name}
                </code>
                <Badge variant="outline" className="w-fit font-mono text-[9.5px] font-normal">
                  {def}
                </Badge>
                <span className="text-[11px] leading-relaxed text-muted-foreground sm:flex-1">
                  {desc}
                </span>
              </div>
            </div>
          ))}
        </div>
      </Panel>

      <Panel title="安全与使用说明" icon={<ShieldCheck className="size-3" />}>
        <div className="space-y-2.5">
          <Note icon={<Lock className="size-3" />} title="凭据加密">
            账号密码、2FA 密钥与应用专用密码使用 AES-256-GCM 加密后写入 SQLite。密钥来自
            GM_SECRET，未设置时自动生成并保存在数据目录的 secret.key，请随数据库一同备份。
          </Note>
          <Note icon={<KeyRound className="size-3" />} title="应用专用密码">
            Gmail IMAP 必须使用 16 位应用专用密码（需先开启两步验证）。账号主密码无法用于 IMAP 登录。
          </Note>
          <Note icon={<Mail className="size-3" />} title="取件范围">
            默认同时读取收件箱、垃圾邮件与已删除邮件，按时间倒序合并。单账号取件返回最新 10 封，
            批量取件每个账号返回最新 1 封。
          </Note>
          <Note icon={<Database className="size-3" />} title="部署">
            前后端打包在同一容器内，Go 进程同时提供静态页面与 /api 接口。数据库位于 /data 卷，
            重建容器不会丢失数据。
          </Note>
        </div>
      </Panel>

      <Panel title="账号导入格式" icon={<Mail className="size-3" />}>
        <pre className="m-0 overflow-x-auto rounded-lg border border-border bg-muted/40 p-3 font-mono text-[11px] leading-relaxed whitespace-pre-wrap">
{`邮箱----密码----2FA密钥----应用专用密码
alice@gmail.com----MyPass123!----JBSWY3DPEHPK3PXP----abcd efgh ijkl mnop
bob@gmail.com----OtherPass----KRSXG5CTMVRXEZLU----qrstuvwxyzabcdef`}
        </pre>
        <p className="mt-2 text-[10.5px] text-muted-foreground">
          也支持 Tab、竖线或逗号分隔；2FA 列留空表示未配置；应用密码中的空格会自动移除。
        </p>
      </Panel>
    </div>
  )
}

function Panel({
  title,
  icon,
  children,
}: {
  title: string
  icon?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card shadow-xs">
      <div className="flex items-center gap-1.5 border-b border-border bg-muted/40 px-3 py-2">
        <span className="text-muted-foreground">{icon}</span>
        <span className="text-[10px] font-semibold tracking-[0.07em] text-muted-foreground uppercase">
          {title}
        </span>
      </div>
      <div className="p-3">{children}</div>
    </div>
  )
}

function Note({
  icon,
  title,
  children,
}: {
  icon: React.ReactNode
  title: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-start gap-2.5">
      <span className="mt-0.5 grid size-5 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">
        {icon}
      </span>
      <div className="min-w-0">
        <p className="text-[11.5px] font-medium">{title}</p>
        <p className="text-[11px] leading-relaxed text-muted-foreground">{children}</p>
      </div>
    </div>
  )
}
