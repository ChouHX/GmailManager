import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowLeft,
  Copy,
  Download,
  FileText,
  Inbox,
  Loader2,
  Mail,
  MailOpen,
  RefreshCw,
  Search,
  Square,
} from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { api, ApiError, type Account, type FetchResult } from '../api'
import { useToast } from '../components/ToastProvider'
import { PageHeader, EmptyState } from '../components/PageHeader'
import { MessageDialog } from './MessageDialog'
import { copyText, formatDateTime, relativeTime } from '../utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Pagination } from '@/components/ui/pagination'
import { Separator } from '@/components/ui/separator'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import Loader from '@/components/kokonutui/loader'
import { StatusIsland, type IslandTask } from '@/components/kokonutui/status-island'

const CONCURRENCY = 5

const FOLDERS = [
  { value: 'auto', label: '收件箱 + 垃圾箱' },
  { value: 'inbox', label: '仅收件箱' },
  { value: 'spam', label: '仅垃圾箱' },
  { value: 'trash', label: '仅已删除' },
]

export function FetchPage() {
  const { toast } = useToast()
  const [rows, setRows] = useState<Account[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [query, setQuery] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [folder, setFolder] = useState('auto')
  const [selected, setSelected] = useState<number[]>([])
  const [running, setRunning] = useState(false)
  const [tasks, setTasks] = useState<IslandTask[]>([])
  const [rowBusy, setRowBusy] = useState<number[]>([])
  const [dismissed, setDismissed] = useState<string[]>([])

  const [dialog, setDialog] = useState<{
    open: boolean
    accountId: number | null
    email: string
    result: FetchResult | null
    loading: boolean
    error: string
  }>({ open: false, accountId: null, email: '', result: null, loading: false, error: '' })

  const abortRef = useRef(false)
  const pageCount = Math.max(1, Math.ceil(total / pageSize))

  const load = useCallback(
    async (silent = false) => {
      if (!silent) setLoading(true)
      setError('')
      try {
        const res = await api.listAccounts(page, pageSize, query)
        setRows(res.items ?? [])
        setTotal(res.total)
        setSelected((prev) => prev.filter((id) => (res.items ?? []).some((r) => r.id === id)))
      } catch (err) {
        setError(err instanceof ApiError ? err.message : '加载失败')
      } finally {
        setLoading(false)
      }
    },
    [page, pageSize, query],
  )

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    const t = setTimeout(() => {
      setPage(1)
      setQuery(searchInput.trim())
    }, 350)
    return () => clearTimeout(t)
  }, [searchInput])

  useEffect(
    () => () => {
      abortRef.current = true
    },
    [],
  )

  const allSelected = rows.length > 0 && rows.every((r) => selected.includes(r.id))
  const targets = selected.length > 0 ? rows.filter((r) => selected.includes(r.id)) : rows

  const applyResult = useCallback((res: FetchResult) => {
    setRows((prev) =>
      prev.map((row) => {
        if (row.id !== res.accountId) return row
        const top = res.messages?.[0]
        return {
          ...row,
          lastFetchAt: res.fetchedAt || Math.floor(Date.now() / 1000),
          lastFetchStatus: res.ok ? 'ok' : 'error',
          lastFetchError: res.error ?? '',
          lastFetchSubject: res.ok && top ? top.subject : row.lastFetchSubject,
          lastFetchFrom: res.ok && top ? top.from : row.lastFetchFrom,
          lastFetchDate: res.ok && top ? top.date : row.lastFetchDate,
          lastFetchSeen: res.ok && top ? top.seen : row.lastFetchSeen,
        }
      }),
    )
  }, [])

  /** Batch fetch: newest message per account, 5 concurrent client requests. */
  const runBatch = async () => {
    if (!targets.length || running) return
    abortRef.current = false
    setRunning(true)
    setTasks([])
    setDismissed([])

    let cursor = 0
    let ok = 0
    let fail = 0
    const totalN = targets.length

    const pushTask = (task: IslandTask) =>
      setTasks((prev) => {
        const next = prev.filter((t) => t.id !== task.id)
        return [...next, task]
      })

    const worker = async () => {
      for (;;) {
        if (abortRef.current) return
        const index = cursor++
        if (index >= totalN) return
        const account = targets[index]
        pushTask({
          id: `acc-${account.id}`,
          label: account.email,
          detail: '取件中',
          status: 'running',
          progress: (ok + fail) / totalN,
        })
        try {
          const res = await api.fetchAccount(account.id, 1, folder)
          applyResult(res)
          if (res.ok) ok++
          else fail++
          pushTask({
            id: `acc-${account.id}`,
            label: account.email,
            detail: res.ok ? res.messages?.[0]?.subject ?? '无新邮件' : res.error ?? '失败',
            status: res.ok ? 'done' : 'error',
          })
        } catch (err) {
          fail++
          const message = err instanceof ApiError ? err.message : '取件失败'
          setRows((prev) =>
            prev.map((row) =>
              row.id === account.id
                ? {
                    ...row,
                    lastFetchAt: Math.floor(Date.now() / 1000),
                    lastFetchStatus: 'error',
                    lastFetchError: message,
                  }
                : row,
            ),
          )
          pushTask({ id: `acc-${account.id}`, label: account.email, detail: message, status: 'error' })
        }
      }
    }

    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, totalN) }, () => worker()))
    setRunning(false)
    pushTask({
      id: 'summary',
      label: `批量取件完成`,
      detail: `成功 ${ok} · 失败 ${fail}`,
      status: fail === 0 ? 'done' : fail === totalN ? 'error' : 'done',
    })
    toast(
      fail === 0 ? `批量取件完成：${ok} 个账号成功` : `批量取件完成：成功 ${ok}，失败 ${fail}`,
      fail === 0 ? 'success' : fail === totalN ? 'error' : 'warning',
    )
  }

  /** Single fetch: newest 10 messages in a dialog. */
  const runSingle = async (account: Account) => {
    if (rowBusy.includes(account.id)) return
    setRowBusy((prev) => [...prev, account.id])
    setDialog({ open: true, accountId: account.id, email: account.email, result: null, loading: true, error: '' })
    try {
      const res = await api.fetchAccount(account.id, 10, folder)
      applyResult(res)
      setDialog((d) => ({ ...d, result: res, loading: false, error: res.ok ? '' : res.error ?? '取件失败' }))
    } catch (err) {
      setDialog((d) => ({
        ...d,
        loading: false,
        error: err instanceof ApiError ? err.message : '取件失败',
      }))
    } finally {
      setRowBusy((prev) => prev.filter((id) => id !== account.id))
    }
  }

  const reloadDialog = async () => {
    const id = dialog.accountId
    if (!id) return
    setDialog((d) => ({ ...d, loading: true, error: '' }))
    try {
      const res = await api.fetchAccount(id, 10, folder)
      applyResult(res)
      setDialog((d) => ({ ...d, result: res, loading: false, error: res.ok ? '' : res.error ?? '取件失败' }))
    } catch (err) {
      setDialog((d) => ({ ...d, loading: false, error: err instanceof ApiError ? err.message : '取件失败' }))
    }
  }

  const copySecret = async (value: string) => {
    const ok = await copyText(value)
    toast(ok ? '应用密码已复制' : '复制失败', ok ? 'success' : 'error')
  }

  const folderLabel = useMemo(
    () => FOLDERS.find((f) => f.value === folder)?.label ?? '',
    [folder],
  )

  return (
    <div className="space-y-3">
      <PageHeader
        title="账号取件"
        subtitle={`通过 IMAP 使用应用专用密码取件 · 当前范围：${folderLabel}`}
        meta={<Badge variant="muted">{total} 个账号</Badge>}
        actions={
          <>
            <Select value={folder} onValueChange={setFolder}>
              <SelectTrigger size="sm" className="w-[148px] text-[11.5px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FOLDERS.map((f) => (
                  <SelectItem key={f.value} value={f.value} className="text-[11.5px]">
                    {f.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {running ? (
              <Button size="sm" variant="outline" onClick={() => {
                abortRef.current = true
                toast('已停止后续取件', 'info')
              }}>
                <Square className="size-3" />
                停止
              </Button>
            ) : null}

            <Button size="sm" onClick={() => void runBatch()} disabled={running || targets.length === 0}>
              {running ? <Loader2 className="size-3.5 animate-spin" /> : <Download className="size-3.5" />}
              {running
                ? '取件中…'
                : selected.length
                  ? `批量取件 (${selected.length})`
                  : `批量取件 (${rows.length})`}
            </Button>
          </>
        }
      />

      <div className="overflow-hidden rounded-xl border border-border bg-card shadow-xs">
        <div className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2">
          <div className="relative w-full sm:max-w-[260px]">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground/60" />
            <Input
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="搜索邮箱或备注"
              className="h-8 pl-8 text-xs sm:h-7"
            />
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button variant="ghost" size="icon-sm" onClick={() => void load()} disabled={running} aria-label="刷新列表">
                  <RefreshCw className="size-3.5" />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>刷新列表</TooltipContent>
          </Tooltip>
          <div className="hidden flex-1 sm:block" />
          <span className="text-[10.5px] text-muted-foreground">
            {selected.length ? `已选 ${selected.length} 个，批量取件仅作用于所选` : '未选择时批量取件作用于当前页'}
          </span>
        </div>

        {error && (
          <div className="border-b border-border bg-destructive/6 px-3 py-2 text-[11.5px] text-destructive">
            {error}
          </div>
        )}

        {/* Phones: card list instead of the wide table. */}
        <div className="divide-y divide-border md:hidden">
          {loading && rows.length === 0 && (
            <Loader title="正在加载账号…" subtitle="读取本地数据库" size="sm" className="gap-3 py-8" />
          )}
          {!loading && rows.length === 0 && (
            <EmptyState icon={<Inbox className="size-6" />} title="暂无账号" hint="请先在「账号管理」中导入账号" />
          )}
          {rows.map((row) => {
            const busy = rowBusy.includes(row.id)
            return (
              <div
                key={row.id}
                data-state={selected.includes(row.id) ? 'selected' : undefined}
                className="px-3 py-2.5 data-[state=selected]:bg-primary/[0.055]"
              >
                <div className="flex items-start gap-2">
                  <Checkbox
                    checked={selected.includes(row.id)}
                    onCheckedChange={() =>
                      setSelected((prev) =>
                        prev.includes(row.id) ? prev.filter((x) => x !== row.id) : [...prev, row.id],
                      )
                    }
                    aria-label={`选择 ${row.email}`}
                    className="mt-1"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      <span className="truncate text-[12.5px] font-medium">{row.email}</span>
                      {!row.hasAppPassword && <Badge variant="warning">缺少密码</Badge>}
                    </div>
                    <div className="mt-1 flex min-w-0 items-center gap-1.5 text-[10.5px]">
                      {row.lastFetchStatus === 'error' ? (
                        <span className="line-clamp-2 text-destructive">
                          {row.lastFetchError || '取件失败'}
                        </span>
                      ) : row.lastFetchAt ? (
                        <>
                          {row.lastFetchSeen ? (
                            <MailOpen className="size-3 shrink-0 text-muted-foreground/60" />
                          ) : (
                            <Mail className="size-3 shrink-0 text-primary" />
                          )}
                          <span className="truncate text-muted-foreground">
                            {row.lastFetchSubject || '(无主题)'}
                          </span>
                        </>
                      ) : (
                        <span className="text-muted-foreground/70">尚未取件</span>
                      )}
                    </div>
                    <div className="mt-1 flex items-center gap-2 text-[10px] text-muted-foreground/80">
                      <span className="whitespace-nowrap">{relativeTime(row.lastFetchAt)}</span>
                      {row.hasAppPassword && (
                        <button
                          type="button"
                          onClick={() => void copySecret(row.appPassword)}
                          className="inline-flex items-center gap-1 rounded px-1 py-0.5 hover:bg-accent"
                        >
                          <Copy className="size-2.5" />
                          应用密码
                        </button>
                      )}
                    </div>
                  </div>
                  <Button
                    size="xs"
                    variant="outline"
                    className="shrink-0"
                    disabled={busy || running}
                    onClick={() => void runSingle(row)}
                  >
                    {busy ? <Loader2 className="size-3 animate-spin" /> : <Mail className="size-3" />}
                    {busy ? '取件中' : '取件'}
                  </Button>
                </div>
              </div>
            )
          })}
        </div>

        <Table className="hidden min-w-[880px] table-fixed md:table">
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-9">
                <Checkbox
                  checked={allSelected ? true : selected.length > 0 ? 'indeterminate' : false}
                  onCheckedChange={() => setSelected(allSelected ? [] : rows.map((r) => r.id))}
                  disabled={rows.length === 0}
                  aria-label="全选"
                />
              </TableHead>
              <TableHead className="w-[210px]">邮箱</TableHead>
              <TableHead className="w-[140px]">应用密码</TableHead>
              <TableHead className="w-[142px]">最近取件</TableHead>
              <TableHead>最新邮件</TableHead>
              <TableHead className="w-[84px] text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && rows.length === 0 && (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={6} className="py-6">
                  <Loader title="正在加载账号…" subtitle="读取本地数据库" size="sm" className="gap-3 p-0" />
                </TableCell>
              </TableRow>
            )}
            {!loading && rows.length === 0 && (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={6} className="p-0">
                  <EmptyState
                    icon={<Inbox className="size-6" />}
                    title="暂无账号"
                    hint="请先在「账号管理」中导入账号"
                  />
                </TableCell>
              </TableRow>
            )}
            {rows.map((row) => {
              const busy = rowBusy.includes(row.id)
              return (
                <TableRow key={row.id} data-state={selected.includes(row.id) ? 'selected' : undefined}>
                  <TableCell>
                    <Checkbox
                      checked={selected.includes(row.id)}
                      onCheckedChange={() =>
                        setSelected((prev) =>
                          prev.includes(row.id) ? prev.filter((x) => x !== row.id) : [...prev, row.id],
                        )
                      }
                      aria-label={`选择 ${row.email}`}
                    />
                  </TableCell>
                  <TableCell>
                    <div className="flex min-w-0 items-center gap-1.5">
                      <span className="truncate text-[12px] font-medium">{row.email}</span>
                      {!row.hasAppPassword && <Badge variant="warning">缺少密码</Badge>}
                    </div>
                    {row.note && (
                      <p className="truncate text-[10px] text-muted-foreground">{row.note}</p>
                    )}
                  </TableCell>
                  <TableCell>
                    {row.hasAppPassword ? (
                      <div className="flex items-center gap-1">
                        <span className="font-mono text-[10.5px] tracking-[0.06em] text-muted-foreground">
                          •••• •••• ••••
                        </span>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              onClick={() => void copySecret(row.appPassword)}
                              aria-label="复制应用密码"
                              className="rounded p-0.5 text-muted-foreground/60 transition-colors hover:text-foreground"
                            >
                              <Copy className="size-2.5" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent>复制应用密码</TooltipContent>
                        </Tooltip>
                      </div>
                    ) : (
                      <span className="text-[10.5px] text-muted-foreground/70">未设置</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <p className="text-[11px] whitespace-nowrap">{relativeTime(row.lastFetchAt)}</p>
                    <p className="text-[9.5px] whitespace-nowrap text-muted-foreground/80">
                      {row.lastFetchAt ? formatDateTime(row.lastFetchAt) : '—'}
                    </p>
                  </TableCell>
                  <TableCell className="min-w-0">
                    {row.lastFetchStatus === 'error' ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <p className="cursor-help truncate text-[10.5px] text-destructive">
                            {row.lastFetchError || '取件失败'}
                          </p>
                        </TooltipTrigger>
                        <TooltipContent className="max-w-[300px] text-[11px] leading-relaxed">
                          {row.lastFetchError}
                        </TooltipContent>
                      </Tooltip>
                    ) : row.lastFetchAt ? (
                      <div className="flex min-w-0 items-center gap-1.5">
                        {row.lastFetchSeen ? (
                          <MailOpen className="size-3 shrink-0 text-muted-foreground/60" />
                        ) : (
                          <Mail className="size-3 shrink-0 text-primary" />
                        )}
                        <div className="min-w-0">
                          <p className="truncate text-[11.5px]">{row.lastFetchSubject || '(无主题)'}</p>
                          <p className="truncate text-[10px] text-muted-foreground/80">
                            {row.lastFetchFrom || '—'} · {row.lastFetchDate ? formatDateTime(row.lastFetchDate) : ''}
                          </p>
                        </div>
                      </div>
                    ) : (
                      <span className="text-[10.5px] text-muted-foreground/70">尚未取件</span>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={busy || running}
                      onClick={() => void runSingle(row)}
                    >
                      {busy ? <Loader2 className="size-3 animate-spin" /> : <Mail className="size-3" />}
                      {busy ? '取件中' : '取件'}
                    </Button>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>

        <div className="flex flex-col items-center gap-2 border-t border-border px-3 py-2 sm:flex-row sm:justify-between">
          <div className="flex items-center gap-2">
            <span className="text-[10.5px] text-muted-foreground">
              第 {page} / {pageCount} 页 · 共 {total} 条
            </span>
            <Select value={String(pageSize)} onValueChange={(v) => { setPageSize(Number(v)); setPage(1) }}>
              <SelectTrigger size="sm" className="h-6 w-[86px] text-[11px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[10, 20, 50].map((n) => (
                  <SelectItem key={n} value={String(n)} className="text-[11px]">
                    每页 {n}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Pagination page={page} pageCount={pageCount} onChange={setPage} />
        </div>
      </div>

      <MessageDialog
        open={dialog.open}
        accountId={dialog.accountId}
        accountEmail={dialog.email}
        result={dialog.result}
        loading={dialog.loading}
        error={dialog.error}
        onClose={() => setDialog((d) => ({ ...d, open: false }))}
        onReload={() => void reloadDialog()}
      />

      {/* KokonutUI status island reports live batch progress. */}
      <div className="pointer-events-none fixed inset-x-0 bottom-4 z-40 flex justify-center">
        <div className="pointer-events-auto">
          <StatusIsland
            tasks={tasks.filter((t) => !dismissed.includes(t.id))}
            onDismiss={(id) => setDismissed((prev) => [...prev, id])}
            dismissAfter={8000}
          />
        </div>
      </div>

      <AnimatePresence>
        {running && (
          <motion.div
            initial={{ scaleX: 0 }}
            animate={{ scaleX: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-x-0 top-0 z-50 h-0.5 origin-left bg-primary"
            style={{
              animation: 'progress 1.6s ease-in-out infinite',
            }}
          />
        )}
      </AnimatePresence>

      <Separator className="hidden" />
      <span className="hidden">
        <FileText />
        <ArrowLeft />
      </span>
    </div>
  )
}
