import { useCallback, useEffect, useState } from 'react'
import {
  BellRing,
  Loader2,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Trash2,
  Webhook,
  Zap,
} from 'lucide-react'
import { api, ApiError, type Watch, type WatchStatus } from '../api'
import { useToast } from '../components/ToastProvider'
import { EmptyState, PageHeader } from '../components/PageHeader'
import { WatchDialog } from './WatchDialog'
import { formatDateTime, relativeTime } from '../utils'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import Loader from '@/components/kokonutui/loader'

export function WatchesPage() {
  const { toast } = useToast()
  const [items, setItems] = useState<Watch[]>([])
  const [status, setStatus] = useState<WatchStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busyIds, setBusyIds] = useState<number[]>([])
  const [dialog, setDialog] = useState<{ open: boolean; watch: Watch | null }>({
    open: false,
    watch: null,
  })
  const [confirm, setConfirm] = useState<Watch | null>(null)

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true)
    setError('')
    try {
      const res = await api.listWatches()
      setItems(res.items ?? [])
      setStatus(res.status ?? null)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  // Refresh the table periodically so background runs show up without a reload.
  useEffect(() => {
    const t = window.setInterval(() => void load(true), 15000)
    return () => window.clearInterval(t)
  }, [load])

  const withBusy = async (id: number, fn: () => Promise<void>) => {
    setBusyIds((prev) => [...prev, id])
    try {
      await fn()
    } finally {
      setBusyIds((prev) => prev.filter((x) => x !== id))
    }
  }

  const toggle = (watch: Watch) =>
    withBusy(watch.id, async () => {
      try {
        const res = await api.toggleWatch(watch.id)
        setItems((prev) =>
          prev.map((w) => (w.id === watch.id ? { ...w, enabled: res.enabled } : w)),
        )
        toast(res.enabled ? '已启用轮询' : '已停用轮询', 'info')
      } catch (err) {
        toast(err instanceof ApiError ? err.message : '操作失败', 'error')
      }
    })

  const runNow = (watch: Watch) =>
    withBusy(watch.id, async () => {
      try {
        const res = await api.runWatch(watch.id)
        if (!res.ok) {
          toast(res.error || '轮询失败', 'error')
        } else if (res.notified) {
          toast(`命中 ${res.hits?.length ?? 0} 封邮件，已推送 Discord`, 'success')
        } else if ((res.hits?.length ?? 0) > 0) {
          toast(`命中 ${res.hits.length} 封邮件，但未推送`, 'warning')
        } else {
          toast(`已轮询最近 ${res.fetched} 封，无命中`, 'info')
        }
        await load(true)
      } catch (err) {
        toast(err instanceof ApiError ? err.message : '轮询失败', 'error')
      }
    })

  const remove = async () => {
    if (!confirm) return
    const target = confirm
    setConfirm(null)
    await withBusy(target.id, async () => {
      try {
        await api.deleteWatch(target.id)
        setItems((prev) => prev.filter((w) => w.id !== target.id))
        toast('任务已删除', 'success')
      } catch (err) {
        toast(err instanceof ApiError ? err.message : '删除失败', 'error')
      }
    })
  }

  const enabledCount = items.filter((w) => w.enabled).length

  return (
    <div className="space-y-3">
      <PageHeader
        title="定时轮询"
        subtitle="按设定的间隔轮询账号最近 5 封邮件，标题与发件人命中后推送 Discord webhook"
        meta={
          <>
            <Badge variant="muted">{items.length} 个任务</Badge>
            <Badge variant={enabledCount ? 'success' : 'outline'}>{enabledCount} 个运行中</Badge>
            {status?.active ? <Badge variant="warning">{status.active} 个正在轮询</Badge> : null}
          </>
        }
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
              <RefreshCw className={loading ? 'size-3 animate-spin' : 'size-3'} />
              刷新
            </Button>
            <Button size="sm" onClick={() => setDialog({ open: true, watch: null })}>
              <Plus className="size-3.5" />
              新建任务
            </Button>
          </>
        }
      />

      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/8 px-3 py-2 text-[11.5px] text-destructive">
          {error}
        </div>
      )}

      <div className="overflow-hidden rounded-xl border border-border bg-card shadow-xs">
        {loading ? (
          <div className="grid place-items-center py-16">
            <Loader title="加载任务" size="sm" />
          </div>
        ) : items.length === 0 ? (
          <EmptyState
            icon={<Webhook className="size-6" />}
            title="还没有轮询任务"
            hint="创建任务后，系统会按设定间隔自动检查最近 5 封邮件，命中标题或发件人条件时推送到 Discord。"
            action={
              <Button size="sm" onClick={() => setDialog({ open: true, watch: null })}>
                <Plus className="size-3.5" />
                新建任务
              </Button>
            }
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[34px]" />
                <TableHead>任务 / 账号</TableHead>
                <TableHead className="hidden md:table-cell">匹配条件</TableHead>
                <TableHead className="w-[92px]">间隔</TableHead>
                <TableHead className="hidden lg:table-cell w-[110px]">上次轮询</TableHead>
                <TableHead className="w-[92px]">命中</TableHead>
                <TableHead className="w-[120px] text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((w) => {
                const busy = busyIds.includes(w.id)
                return (
                  <TableRow key={w.id}>
                    <TableCell>
                      <Switch
                        checked={w.enabled}
                        disabled={busy}
                        onCheckedChange={() => void toggle(w)}
                        aria-label={w.enabled ? '停用任务' : '启用任务'}
                      />
                    </TableCell>
                    <TableCell className="max-w-[240px]">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate text-[12px] font-medium">
                          {w.name || w.email || '未命名任务'}
                        </span>
                        {w.lastStatus === 'error' && (
                          <Badge variant="destructive">错误</Badge>
                        )}
                        {w.lastStatus === 'ok' && w.lastHitCount > 0 && (
                          <Badge variant="success">命中</Badge>
                        )}
                      </div>
                      <p className="truncate text-[10.5px] text-muted-foreground">
                        {w.email || `账号 #${w.accountId}`}
                        {w.folder ? ` · ${folderLabel(w.folder)}` : ''}
                      </p>
                      {w.lastError ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <p className="mt-0.5 truncate text-[10px] text-destructive">
                              {w.lastError}
                            </p>
                          </TooltipTrigger>
                          <TooltipContent className="max-w-[320px] break-words">
                            {w.lastError}
                          </TooltipContent>
                        </Tooltip>
                      ) : null}
                    </TableCell>
                    <TableCell className="hidden md:table-cell">
                      <div className="space-y-0.5">
                        <ConditionLine label="标题" value={w.subject} />
                        <ConditionLine label="发件人" value={w.from} />
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className="font-mono text-[11px]">{formatInterval(w.intervalSeconds)}</span>
                    </TableCell>
                    <TableCell className="hidden lg:table-cell">
                      <span className="text-[11px] text-muted-foreground">
                        {w.lastRunAt ? relativeTime(w.lastRunAt) : '尚未运行'}
                      </span>
                      {w.lastRunAt ? (
                        <p className="text-[10px] text-muted-foreground/70">
                          {formatDateTime(w.lastRunAt)}
                        </p>
                      ) : null}
                    </TableCell>
                    <TableCell>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="inline-flex items-center gap-1 text-[11.5px] tabular-nums">
                            <Zap className="size-3 text-amber-500" />
                            {w.totalHits}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          最近一次命中 {w.lastHitCount} 封
                          {w.lastNotifiedAt ? ` · 最近推送 ${formatDateTime(w.lastNotifiedAt)}` : ''}
                        </TooltipContent>
                      </Tooltip>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="outline"
                              size="icon-xs"
                              disabled={busy}
                              onClick={() => void runNow(w)}
                              aria-label="立即轮询"
                            >
                              {busy ? (
                                <Loader2 className="size-3 animate-spin" />
                              ) : (
                                <Play className="size-3" />
                              )}
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>立即轮询一次</TooltipContent>
                        </Tooltip>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          disabled={busy}
                          onClick={() => setDialog({ open: true, watch: w })}
                          aria-label="编辑任务"
                        >
                          <Pencil className="size-3" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          disabled={busy}
                          onClick={() => setConfirm(w)}
                          aria-label="删除任务"
                          className="text-destructive hover:text-destructive"
                        >
                          <Trash2 className="size-3" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        )}
      </div>

      {status && (
        <p className="flex items-center gap-1.5 text-[10.5px] text-muted-foreground">
          <BellRing className="size-3" />
          轮询器运行中 · 每 5 秒检查一次到期任务
          {status.lastTick ? ` · 最近检查 ${relativeTime(Math.floor(new Date(status.lastTick).getTime() / 1000))}` : ''}
        </p>
      )}

      <WatchDialog
        open={dialog.open}
        watch={dialog.watch}
        onClose={() => setDialog({ open: false, watch: null })}
        onSaved={() => void load(true)}
      />

      <Dialog open={Boolean(confirm)} onOpenChange={(v) => !v && setConfirm(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>删除轮询任务</DialogTitle>
            <DialogDescription>
              确认删除「{confirm?.name || confirm?.email}」？删除后不再轮询该任务。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setConfirm(null)}>
              取消
            </Button>
            <Button variant="destructive" size="sm" onClick={() => void remove()}>
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ConditionLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span className="w-9 shrink-0 text-[10px] text-muted-foreground">{label}</span>
      <code className="truncate font-mono text-[10.5px] text-foreground/85">
        {value || '不限'}
      </code>
    </div>
  )
}

function folderLabel(value: string): string {
  switch (value) {
    case 'inbox':
      return '收件箱'
    case 'spam':
      return '垃圾箱'
    case 'trash':
      return '已删除'
    case 'auto':
      return '收件箱+垃圾箱'
    default:
      return value
  }
}

function formatInterval(seconds: number): string {
  if (!seconds || seconds < 60) return `${seconds || 0} 秒`
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`
  return `${seconds} 秒`
}
