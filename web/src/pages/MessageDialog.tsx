import { useEffect, useMemo, useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import {
  ArrowLeft,
  Copy,
  FileText,
  Inbox,
  Loader2,
  Mail,
  MailOpen,
  Paperclip,
  RefreshCw,
  Search,
} from 'lucide-react'
import { api, ApiError, type FetchResult, type MessageDetail, type MessageSummary } from '../api'
import { copyText, formatBytes, formatDate, formatDateTime } from '../utils'
import { cn } from '@/lib/utils'
import { useToast } from '../components/ToastProvider'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import Loader from '@/components/kokonutui/loader'

interface MessageDialogProps {
  open: boolean
  accountId: number | null
  accountEmail: string
  result: FetchResult | null
  loading: boolean
  error: string
  onClose: () => void
  onReload: () => void
}

export function MessageDialog({
  open,
  accountId,
  accountEmail,
  result,
  loading,
  error,
  onClose,
  onReload,
}: MessageDialogProps) {
  const { toast } = useToast()
  const [active, setActive] = useState<MessageSummary | null>(null)
  const [detail, setDetail] = useState<MessageDetail | null>(null)
  const [detailBusy, setDetailBusy] = useState(false)
  const [detailError, setDetailError] = useState('')
  const [filter, setFilter] = useState('')
  const [view, setView] = useState<'html' | 'text'>('html')

  useEffect(() => {
    if (!open) {
      setActive(null)
      setDetail(null)
      setDetailError('')
      setFilter('')
      setView('html')
    }
  }, [open])

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

  const openMessage = async (msg: MessageSummary) => {
    if (!accountId) return
    setActive(msg)
    setDetail(null)
    setDetailError('')
    setDetailBusy(true)
    setView('html')
    try {
      const d = await api.message(accountId, msg.mailbox, msg.uid)
      setDetail(d)
      if (!d.html) setView('text')
    } catch (err) {
      setDetailError(err instanceof ApiError ? err.message : '加载邮件失败')
    } finally {
      setDetailBusy(false)
    }
  }

  const copyAll = async () => {
    if (!detail) return
    const text = [
      `主题: ${detail.subject}`,
      `发件人: ${detail.fromName ? `${detail.fromName} <${detail.from}>` : detail.from}`,
      `收件人: ${detail.to}`,
      `时间: ${formatDateTime(detail.date)}`,
      '',
      detail.text || '',
    ].join('\n')
    const ok = await copyText(text)
    toast(ok ? '邮件内容已复制' : '复制失败', ok ? 'success' : 'error')
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="flex h-[92dvh] flex-col gap-0 p-0 sm:h-[80vh] sm:max-w-3xl">
        <DialogHeader className="flex-row items-center gap-2 space-y-0 pr-10 sm:pr-12">
          {active && (
            <Button variant="ghost" size="icon-xs" onClick={() => setActive(null)} aria-label="返回列表">
              <ArrowLeft className="size-3.5" />
            </Button>
          )}
          <div className="min-w-0 flex-1">
            <DialogTitle className="truncate">
              {active ? active.subject || '(无主题)' : '取件结果'}
            </DialogTitle>
            <DialogDescription className="truncate">
              {accountEmail}
              {!active && result
                ? ` · ${result.messages?.length ?? 0} 封邮件 · ${result.durationMs}ms`
                : ''}
            </DialogDescription>
          </div>
          {active ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="icon-xs" onClick={copyAll} disabled={!detail} aria-label="复制全文">
                  <Copy className="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>复制全文</TooltipContent>
            </Tooltip>
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="icon-xs" onClick={onReload} disabled={loading} aria-label="重新取件">
                  <RefreshCw className={cn('size-3.5', loading && 'animate-spin')} />
                </Button>
              </TooltipTrigger>
              <TooltipContent>重新取件</TooltipContent>
            </Tooltip>
          )}
        </DialogHeader>

        {error && (
          <div className="border-b border-border bg-destructive/6 px-4 py-2 text-[11.5px] text-destructive">
            {error}
          </div>
        )}

        {active ? (
          <MessageDetailView
            detail={detail}
            busy={detailBusy}
            error={detailError}
            view={view}
            onViewChange={setView}
          />
        ) : (
          <>
            <div className="flex items-center gap-2 border-b border-border px-4 py-2">
              <div className="relative w-full max-w-[280px]">
                <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground/60" />
                <Input
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                  placeholder="筛选主题 / 发件人 / 摘要"
                  className="h-7 pl-8 text-xs"
                />
              </div>
              <div className="flex-1" />
              <span className="text-[10.5px] text-muted-foreground">
                {messages.length} / {result?.messages?.length ?? 0}
              </span>
            </div>

            <div className="flex-1 overflow-auto">
              {loading && (
                <Loader title="正在通过 IMAP 取件…" subtitle="连接 Gmail 并读取邮件" size="sm" className="py-10" />
              )}
              {!loading && messages.length === 0 && !error && (
                <div className="flex flex-col items-center gap-1 px-4 py-14 text-center">
                  <Inbox className="mb-1 size-6 text-muted-foreground/40" />
                  <p className="text-[12.5px] font-medium">没有邮件</p>
                  <p className="text-[10.5px] text-muted-foreground">
                    该账号在所选范围内暂无邮件，或应用专用密码无效
                  </p>
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
                            'max-w-[320px] truncate text-[11.5px]',
                            msg.seen ? 'text-muted-foreground' : 'font-semibold text-foreground',
                          )}
                        >
                          {msg.fromName || msg.from || '(未知发件人)'}
                        </span>
                        <Badge variant={msg.folderLabel === '垃圾邮件' ? 'warning' : 'muted'}>
                          {msg.folderLabel || msg.mailbox}
                        </Badge>
                        {msg.hasAttachment && (
                          <Paperclip className="size-3 text-muted-foreground/60" />
                        )}
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

        <DialogFooter className="flex-row items-center justify-between sm:justify-between">
          <span className="text-[10px] text-muted-foreground/80">
            {result?.fetchedAt ? `取件时间 ${formatDateTime(result.fetchedAt)}` : ''}
          </span>
          <Button variant="outline" size="sm" onClick={onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function MessageDetailView({
  detail,
  busy,
  error,
  view,
  onViewChange,
}: {
  detail: MessageDetail | null
  busy: boolean
  error: string
  view: 'html' | 'text'
  onViewChange: (v: 'html' | 'text') => void
}) {
  if (busy) {
    return <Loader title="正在读取邮件正文…" subtitle="解析 MIME 内容" size="sm" className="py-12" />
  }
  if (error) {
    return (
      <div className="m-4 rounded-md bg-destructive/8 px-3 py-2 text-[11.5px] text-destructive">
        {error}
      </div>
    )
  }
  if (!detail) return null

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
          <MetaRow label="位置" value={`${detail.folderLabel || ''} · ${formatBytes(detail.size)}`} />
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
            <TabsTrigger value="html" disabled={!hasHtml} className="h-6 gap-1 text-[11px]">
              <FileText className="size-3" />
              HTML
            </TabsTrigger>
            <TabsTrigger value="text" className="h-6 gap-1 text-[11px]">
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
            // HTML is sanitised server-side with a bluemonday allowlist.
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

/** Small read/unread marker reused by the fetch table. */
export function ReadMark({ seen }: { seen: boolean }) {
  return seen ? (
    <MailOpen className="size-3.5 text-muted-foreground/50" />
  ) : (
    <Mail className="size-3.5 text-primary" />
  )
}

export { Loader2 }
