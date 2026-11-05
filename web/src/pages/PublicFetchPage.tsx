import { useCallback, useEffect, useMemo, useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import {
  AlertCircle,
  ArrowLeft,
  Copy,
  Inbox,
  Loader2,
  Mail,
  MailOpen,
  Paperclip,
  RefreshCw,
  Search,
} from 'lucide-react'
import {
  Copy as CopyIcon,
  ShieldCheck,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import Loader from '@/components/kokonutui/loader'
import { copyText, formatBytes, formatDate, formatDateTime } from '@/utils'

/** Mirrors the server payloads; kept local so the public page has no auth deps. */
interface Summary {
  uid: number
  mailbox: string
  subject: string
  from: string
  fromName: string
  to: string
  date: number
  size: number
  seen: boolean
  snippet?: string
  hasAttachment: boolean
  folderLabel: string
}

interface Detail extends Summary {
  text: string
  html: string
  attachments: { filename: string; mimeType: string; size: number }[] | null
}

interface FetchResult {
  accountId: number
  email: string
  ok: boolean
  error?: string
  messages: Summary[] | null
  fetchedAt: number
  durationMs: number
}

interface PublicInfo {
  email: string
  note: string
  label: string
  lifetime: string
}

type Phase = 'loading' | 'ready' | 'invalid'

/** Reads the token from /f/<token> in the address bar. */
function tokenFromPath(): string {
  const m = window.location.pathname.match(/\/f\/([^/?#]+)/)
  return m ? decodeURIComponent(m[1]) : ''
}

export function PublicFetchPage() {
  const token = useMemo(tokenFromPath, [])
  const [phase, setPhase] = useState<Phase>('loading')
  const [info, setInfo] = useState<PublicInfo | null>(null)
  const [result, setResult] = useState<FetchResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [active, setActive] = useState<Summary | null>(null)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [detailBusy, setDetailBusy] = useState(false)
  const [detailError, setDetailError] = useState('')
  const [view, setView] = useState<'html' | 'text'>('html')
  const [filter, setFilter] = useState('')

  const load = useCallback(
    async (silent = false) => {
      if (!token) {
        setPhase('invalid')
        return
      }
      if (!silent) setBusy(true)
      setError('')
      try {
        const infoRes = await fetch(`/api/public/f/${encodeURIComponent(token)}`)
        if (infoRes.status === 404) {
          setPhase('invalid')
          return
        }
        if (!infoRes.ok) throw new Error(`加载失败 (${infoRes.status})`)
        setInfo(await infoRes.json())

        const res = await fetch(
          `/api/public/f/${encodeURIComponent(token)}/messages?limit=10&folder=auto`,
        )
        if (!res.ok) throw new Error(`取件失败 (${res.status})`)
        const data: FetchResult = await res.json()
        setResult(data)
        if (!data.ok) setError(data.error || '取件失败')
        setPhase('ready')
      } catch (err) {
        setError(err instanceof Error ? err.message : '加载失败')
        setPhase((p) => (p === 'loading' ? 'invalid' : p))
      } finally {
        setBusy(false)
      }
    },
    [token],
  )

  useEffect(() => {
    void load()
  }, [load])

  const openMessage = async (msg: Summary) => {
    setActive(msg)
    setDetail(null)
    setDetailError('')
    setDetailBusy(true)
    setView('html')
    try {
      const res = await fetch(
        `/api/public/f/${encodeURIComponent(token)}/message?uid=${msg.uid}&mailbox=${encodeURIComponent(msg.mailbox)}`,
      )
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        throw new Error(body?.error || `读取失败 (${res.status})`)
      }
      const d: Detail = await res.json()
      setDetail(d)
      if (!d.html) setView('text')
    } catch (err) {
      setDetailError(err instanceof Error ? err.message : '读取失败')
    } finally {
      setDetailBusy(false)
    }
  }

  const messages = useMemo(() => {
    const list = result?.messages ?? []
    if (!filter.trim()) return list
    const q = filter.trim().toLowerCase()
    return list.filter(
      (m) =>
        m.subject.toLowerCase().includes(q) ||
        m.from.toLowerCase().includes(q) ||
        m.fromName.toLowerCase().includes(q) ||
        m.snippet?.toLowerCase().includes(q),
    )
  }, [result, filter])

  const copyLink = async () => {
    const ok = await copyText(window.location.href)
    if (ok) window.setTimeout(() => undefined, 0)
  }

  if (phase === 'invalid') {
    return (
      <Shell>
        <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
          <AlertCircle className="mb-1 size-7 text-muted-foreground/40" />
          <p className="text-[13.5px] font-medium">链接无效或已失效</p>
          <p className="max-w-xs text-[11.5px] leading-relaxed text-muted-foreground">
            该取件链接可能已被撤销，请联系管理员重新生成。
          </p>
        </div>
      </Shell>
    )
  }

  return (
    <Shell>
      <header className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2.5">
        {active ? (
          <Button variant="ghost" size="icon-xs" onClick={() => setActive(null)} aria-label="返回列表">
            <ArrowLeft className="size-3.5" />
          </Button>
        ) : null}
        <div className="min-w-0 flex-1">
          <p className="truncate text-[12.5px] font-semibold">
            {active ? active.subject || '(无主题)' : info?.email ?? '正在加载…'}
          </p>
          <p className="truncate text-[10.5px] text-muted-foreground">
            {active
              ? `${active.folderLabel || ''} · ${formatDateTime(active.date)}`
              : result
                ? `最新 ${result.messages?.length ?? 0} 封 · ${result.durationMs}ms`
                : '只读取件页面'}
          </p>
        </div>
        {active ? (
          <Button variant="ghost" size="icon-xs" onClick={copyLink} aria-label="复制当前链接">
            <Copy className="size-3.5" />
          </Button>
        ) : (
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => void load()}
            disabled={busy}
            aria-label="刷新"
          >
            <RefreshCw className={cn('size-3.5', busy && 'animate-spin')} />
          </Button>
        )}
      </header>

      {error && (
        <div className="border-b border-border bg-destructive/6 px-4 py-2 text-[11.5px] text-destructive">
          {error}
        </div>
      )}

      {active ? (
        <DetailView
          detail={detail}
          busy={detailBusy}
          error={detailError}
          view={view}
          onViewChange={setView}
        />
      ) : (
        <>
          <div className="flex items-center gap-2 border-b border-border px-4 py-2">
            <div className="relative w-full sm:max-w-[260px]">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground/60" />
              <Input
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder="筛选主题 / 发件人"
                className="h-8 pl-8 text-xs"
              />
            </div>
            <div className="flex-1" />
            <span className="text-[10.5px] text-muted-foreground">
              {messages.length} / {result?.messages?.length ?? 0}
            </span>
          </div>

          <div className="min-h-0 flex-1 overflow-auto">
            {phase === 'loading' && (
              <Loader title="正在取件…" subtitle="读取最新邮件" size="sm" className="py-12" />
            )}
            {phase === 'ready' && messages.length === 0 && !error && (
              <div className="flex flex-col items-center gap-1 px-4 py-14 text-center">
                <Inbox className="mb-1 size-6 text-muted-foreground/40" />
                <p className="text-[12.5px] font-medium">没有邮件</p>
                <p className="text-[10.5px] text-muted-foreground">该账号暂无最新邮件</p>
              </div>
            )}
            <AnimatePresence initial={false}>
              {messages.map((msg, i) => (
                <motion.button
                  key={`${msg.mailbox}-${msg.uid}`}
                  type="button"
                  onClick={() => void openMessage(msg)}
                  initial={{ opacity: 0, y: 4 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: Math.min(i * 0.025, 0.2), duration: 0.2 }}
                  className="flex w-full items-start gap-2.5 border-b border-border px-4 py-2.5 text-left transition-colors last:border-0 hover:bg-accent/50"
                >
                  <span className="mt-1.5 shrink-0">
                    {msg.seen ? (
                      <MailOpen className="size-3.5 text-muted-foreground/50" />
                    ) : (
                      <span className="block size-2 rounded-full bg-primary" />
                    )}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-1.5">
                      <span
                        className={cn(
                          'max-w-[240px] truncate text-[11.5px]',
                          msg.seen ? 'text-muted-foreground' : 'font-semibold text-foreground',
                        )}
                      >
                        {msg.fromName || msg.from || '(未知发件人)'}
                      </span>
                      <Badge variant={msg.folderLabel === '垃圾邮件' ? 'warning' : 'muted'}>
                        {msg.folderLabel || msg.mailbox}
                      </Badge>
                      {msg.hasAttachment && <Paperclip className="size-3 text-muted-foreground/60" />}
                      <span className="flex-1" />
                      <span className="shrink-0 text-[10px] text-muted-foreground/80">
                        {formatDate(msg.date)}
                      </span>
                    </span>
                    <span
                      className={cn(
                        'mt-0.5 block truncate text-[11.5px]',
                        msg.seen ? 'font-normal' : 'font-medium',
                      )}
                    >
                      {msg.subject || '(无主题)'}
                    </span>
                    <span className="mt-0.5 block truncate text-[10.5px] text-muted-foreground/80">
                      {msg.snippet || msg.to || '—'}
                    </span>
                  </span>
                </motion.button>
              ))}
            </AnimatePresence>
          </div>
        </>
      )}

      <footer className="flex items-center justify-between gap-2 border-t border-border px-4 py-2">
        <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground/80">
          <ShieldCheck className="size-2.5" />
          只读 · 仅该账号最新邮件
        </span>
        <Button variant="ghost" size="xs" onClick={copyLink}>
          <CopyIcon className="size-3" />
          复制链接
        </Button>
      </footer>
    </Shell>
  )
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-full flex-col bg-background">
      <div className="flex flex-1 flex-col overflow-hidden border-border bg-card sm:mx-auto sm:my-6 sm:max-w-3xl sm:rounded-xl sm:border sm:shadow-sm">
        {children}
      </div>
    </div>
  )
}

function DetailView({
  detail,
  busy,
  error,
  view,
  onViewChange,
}: {
  detail: Detail | null
  busy: boolean
  error: string
  view: 'html' | 'text'
  onViewChange: (v: 'html' | 'text') => void
}) {
  if (busy) {
    return <Loader title="正在读取邮件…" subtitle="解析 MIME 内容" size="sm" className="py-12" />
  }
  if (error) {
    return (
      <div className="m-4 rounded-md bg-destructive/8 px-3 py-2 text-[11.5px] text-destructive">
        {error}
      </div>
    )
  }
  if (!detail) return <div className="flex-1" />

  const hasHtml = Boolean(detail.html)
  const attachments = detail.attachments ?? []

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-b border-border bg-muted/35 px-4 py-3">
        <p className="mb-1.5 text-[13px] font-semibold">{detail.subject || '(无主题)'}</p>
        <div className="grid grid-cols-1 gap-x-6 gap-y-0.5 sm:grid-cols-2">
          <MetaRow label="发件人" value={detail.fromName ? `${detail.fromName} <${detail.from}>` : detail.from} />
          <MetaRow label="时间" value={formatDateTime(detail.date)} />
          <MetaRow label="收件人" value={detail.to || '—'} />
          <MetaRow label="大小" value={formatBytes(detail.size)} />
        </div>
      </div>

      {attachments.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 border-b border-border px-4 py-2">
          <span className="text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">
            附件 {attachments.length}
          </span>
          {attachments.map((a, i) => (
            <Badge key={`${a.filename}-${i}`} variant="outline" className="gap-1 font-normal">
              <Paperclip className="size-2.5" />
              {a.filename} · {formatBytes(a.size)}
            </Badge>
          ))}
        </div>
      )}

      <div className="border-b border-border px-3 py-1.5">
        <Tabs value={view} onValueChange={(v) => onViewChange(v as 'html' | 'text')}>
          <TabsList className="h-7">
            <TabsTrigger value="html" disabled={!hasHtml} className="h-6 text-[11px]">
              HTML
            </TabsTrigger>
            <TabsTrigger value="text" className="h-6 text-[11px]">
              纯文本
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      <div
        className={cn(
          'min-h-0 flex-1 overflow-auto p-4',
          view === 'html' && hasHtml ? 'bg-card' : 'bg-muted/30',
        )}
      >
        {view === 'html' && hasHtml ? (
          <div
            className={cn(
              'text-[12.5px] leading-relaxed break-words',
              '[&_img]:h-auto [&_img]:max-w-full',
              '[&_table]:max-w-full [&_table]:border-collapse',
              '[&_a]:text-primary [&_a]:underline [&_a]:underline-offset-2',
              '[&_pre]:text-[11.5px] [&_pre]:whitespace-pre-wrap',
              '[&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground',
            )}
            dangerouslySetInnerHTML={{ __html: detail.html }}
          />
        ) : (
          <pre className="m-0 font-mono text-[11.5px] leading-relaxed break-words whitespace-pre-wrap">
            {detail.text || '(无正文内容)'}
          </pre>
        )}
      </div>
    </div>
  )
}

function MetaRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 gap-2">
      <span className="min-w-10 shrink-0 text-[10px] text-muted-foreground/80">{label}</span>
      <span className="truncate text-[11px]">{value || '—'}</span>
    </div>
  )
}

/** Kept for parity with the admin page icon set. */
export const PublicMailIcon = Mail
export { Loader2 }
