import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Check,
  Copy,
  Eye,
  EyeOff,
  KeyRound,
  Link2,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  Upload,
  Download,
  ExternalLink,
} from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { api, ApiError, type Account, type ImportResult } from '../api'
import { useToast } from '../components/ToastProvider'
import { PageHeader, EmptyState } from '../components/PageHeader'
import { AccountDialog } from './AccountDialog'
import { copyText, formatDateTime, relativeTime } from '../utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Separator } from '@/components/ui/separator'
import { Pagination } from '@/components/ui/pagination'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
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
import Loader from '@/components/kokonutui/loader'

const PAGE_SIZE = 10

export function AccountsPage() {
  const { toast } = useToast()
  const [rows, setRows] = useState<Account[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [query, setQuery] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<number[]>([])

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Account | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Account | null>(null)
  const [bulkDelete, setBulkDelete] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [linkFor, setLinkFor] = useState<Account | null>(null)
  const [linkBusy, setLinkBusy] = useState(false)
  const [bulkLinkBusy, setBulkLinkBusy] = useState(false)
  const [totpFor, setTotpFor] = useState<Account | null>(null)
  const [totp, setTotp] = useState<{ code: string; remaining: number } | null>(null)

  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const load = async (opts?: { silent?: boolean }) => {
    if (!opts?.silent) setLoading(true)
    setError('')
    try {
      const res = await api.listAccounts(page, PAGE_SIZE, query)
      setRows(res.items ?? [])
      setTotal(res.total)
      setSelected((prev) => prev.filter((id) => (res.items ?? []).some((r) => r.id === id)))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, query])

  useEffect(() => {
    const t = setTimeout(() => {
      setPage(1)
      setQuery(searchInput.trim())
    }, 350)
    return () => clearTimeout(t)
  }, [searchInput])

  useEffect(() => {
    if (!totpFor) return
    const tick = async () => {
      try {
        const res = await api.totp(totpFor.id)
        setTotp({ code: res.code, remaining: res.remaining })
      } catch (err) {
        toast(err instanceof ApiError ? err.message : '获取 2FA 失败', 'error')
        setTotpFor(null)
      }
    }
    void tick()
    const timer = setInterval(() => void tick(), 5000)
    return () => clearInterval(timer)
  }, [totpFor, toast])

  const allSelected = rows.length > 0 && rows.every((r) => selected.includes(r.id))

  const toggleAll = () => setSelected(allSelected ? [] : rows.map((r) => r.id))
  const toggleOne = (id: number) =>
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await api.deleteAccount(deleteTarget.id)
      toast('账号已删除', 'success')
      setDeleteTarget(null)
      if (rows.length === 1 && page > 1) setPage(page - 1)
      else void load({ silent: true })
    } catch (err) {
      toast(err instanceof ApiError ? err.message : '删除失败', 'error')
    }
  }

  const handleBulkDelete = async () => {
    try {
      const res = await api.deleteAccounts(selected)
      toast(`已删除 ${res.deleted} 个账号`, 'success')
      setSelected([])
      setBulkDelete(false)
      setPage(1)
      void load({ silent: true })
    } catch (err) {
      toast(err instanceof ApiError ? err.message : '批量删除失败', 'error')
    }
  }

  const copyValue = async (value: string, label: string) => {
    const ok = await copyText(value)
    toast(ok ? `${label} 已复制` : '复制失败', ok ? 'success' : 'error')
  }

  const exportSelected = () => {
    const a = document.createElement('a')
    a.href = api.exportUrl(selected.length ? selected : undefined)
    a.download = ''
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    toast(selected.length ? `正在导出 ${selected.length} 个账号` : '正在导出全部账号', 'info')
  }

  /** Opens the dialog for one account's share link. */
  const openLink = async (row: Account) => {
    setLinkFor(row)
    setLinkBusy(true)
    try {
      const link = await api.accountLink(row.id)
      setLinkFor({ ...row, shareUrl: link.url } as Account & { shareUrl: string })
    } catch (err) {
      toast(err instanceof ApiError ? err.message : '生成链接失败', 'error')
      setLinkFor(null)
    } finally {
      setLinkBusy(false)
    }
  }

  /** Ensures links for the selection, then downloads them as a text file. */
  const exportLinks = async () => {
    if (!selected.length) {
      toast('请先选择账号', 'info')
      return
    }
    setBulkLinkBusy(true)
    try {
      await api.bulkLinks(selected)
      const a = document.createElement('a')
      a.href = api.exportLinksUrl(selected)
      a.download = ''
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      toast(`已生成并导出 ${selected.length} 个取件链接`, 'success')
    } catch (err) {
      toast(err instanceof ApiError ? err.message : '导出链接失败', 'error')
    } finally {
      setBulkLinkBusy(false)
    }
  }

  return (
    <div className="space-y-3">
      <PageHeader
        title="账号管理"
        subtitle="账号格式：邮箱----密码----2FA密钥----应用专用密码"
        meta={<Badge variant="muted">{total} 条</Badge>}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => setImportOpen(true)}>
              <Upload className="size-3.5" />
              导入
            </Button>
            <Button variant="outline" size="sm" onClick={exportSelected}>
              <Download className="size-3.5" />
              {selected.length ? `导出 (${selected.length})` : '导出全部'}
            </Button>
            <Button
              size="sm"
              onClick={() => {
                setEditing(null)
                setDialogOpen(true)
              }}
            >
              <Plus className="size-3.5" />
              新增账号
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
              <Button variant="ghost" size="icon-sm" onClick={() => void load()} aria-label="刷新">
                <RefreshCw className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>刷新</TooltipContent>
          </Tooltip>
          <div className="flex-1" />
          <AnimatePresence>
            {selected.length > 0 && (
              <motion.div
                initial={{ opacity: 0, x: 6 }}
                animate={{ opacity: 1, x: 0 }}
                exit={{ opacity: 0, x: 6 }}
                className="flex items-center gap-2"
              >
                <span className="text-[11px] text-muted-foreground">已选 {selected.length} 项</span>
                <Button
                  variant="outline"
                  size="xs"
                  disabled={bulkLinkBusy}
                  onClick={() => void exportLinks()}
                >
                  {bulkLinkBusy ? (
                    <Loader2 className="size-3 animate-spin" />
                  ) : (
                    <Link2 className="size-3" />
                  )}
                  导出链接
                </Button>
                <Button variant="outline" size="xs" className="text-destructive" onClick={() => setBulkDelete(true)}>
                  <Trash2 className="size-3" />
                  批量删除
                </Button>
              </motion.div>
            )}
          </AnimatePresence>
        </div>

        {error && (
          <div className="border-b border-border bg-destructive/6 px-3 py-2 text-[11.5px] text-destructive">
            {error}
          </div>
        )}

        {/* Phones: one card per account instead of a 930px-wide table. */}
        <div className="divide-y divide-border md:hidden">
          {loading && rows.length === 0 && (
            <Loader title="正在加载账号…" subtitle="读取本地数据库" size="sm" className="gap-3 py-8" />
          )}
          {!loading && rows.length === 0 && (
            <EmptyState
              icon={<KeyRound className="size-6" />}
              title="暂无账号"
              hint="通过「导入」粘贴账号列表，或点击「新增账号」手动添加"
              action={
                <Button variant="outline" size="sm" onClick={() => setImportOpen(true)}>
                  导入账号
                </Button>
              }
            />
          )}
          {rows.map((row) => (
            <div
              key={row.id}
              data-state={selected.includes(row.id) ? 'selected' : undefined}
              className="px-3 py-2.5 data-[state=selected]:bg-primary/[0.055]"
            >
              <div className="flex items-start gap-2">
                <Checkbox
                  checked={selected.includes(row.id)}
                  onCheckedChange={() => toggleOne(row.id)}
                  aria-label={`选择 ${row.email}`}
                  className="mt-1"
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1">
                    <span className="truncate text-[12.5px] font-medium">{row.email}</span>
                    <IconAction label="复制邮箱" onClick={() => void copyValue(row.email, '邮箱')}>
                      <Copy className="size-3" />
                    </IconAction>
                  </div>
                  {row.note && (
                    <p className="truncate text-[10px] text-muted-foreground">{row.note}</p>
                  )}
                  <div className="mt-1.5 grid grid-cols-2 gap-x-3 gap-y-1 text-[10.5px]">
                    <MobileField label="账号密码">
                      <SecretCell value={row.password} onCopy={() => void copyValue(row.password, '密码')} />
                    </MobileField>
                    <MobileField label="应用密码">
                      <SecretCell
                        value={row.appPassword}
                        onCopy={() => void copyValue(row.appPassword, '应用密码')}
                      />
                    </MobileField>
                    <MobileField label="2FA">
                      {row.hasTotp ? (
                        <button
                          type="button"
                          onClick={() => {
                            setTotp(null)
                            setTotpFor(row)
                          }}
                          className="inline-flex items-center gap-1 text-[11px] text-primary"
                        >
                          <KeyRound className="size-3" />
                          查看
                        </button>
                      ) : (
                        <span className="text-muted-foreground/70">未配置</span>
                      )}
                    </MobileField>
                    <MobileField label="最近取件">
                      <span className="whitespace-nowrap">{relativeTime(row.lastFetchAt)}</span>
                    </MobileField>
                  </div>
                  <div className="mt-1.5 flex items-center gap-2">
                    <FetchStatus account={row} />
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-0.5">
                  <IconAction
                    label="编辑"
                    onClick={() => {
                      setEditing(row)
                      setDialogOpen(true)
                    }}
                  >
                    <Pencil className="size-3.5" />
                  </IconAction>
                  <IconAction label="取件链接" onClick={() => void openLink(row)}>
                    <Link2 className="size-3.5" />
                  </IconAction>
                  <IconAction label="删除" danger onClick={() => setDeleteTarget(row)}>
                    <Trash2 className="size-3.5" />
                  </IconAction>
                </div>
              </div>
            </div>
          ))}
        </div>

        <Table className="hidden min-w-[930px] table-fixed md:table">
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-9">
                <Checkbox
                  checked={allSelected ? true : selected.length > 0 ? 'indeterminate' : false}
                  onCheckedChange={toggleAll}
                  disabled={rows.length === 0}
                  aria-label="全选"
                />
              </TableHead>
              <TableHead className="w-[210px]">邮箱</TableHead>
              <TableHead className="w-[130px]">账号密码</TableHead>
              <TableHead className="w-[116px]">2FA 密钥</TableHead>
              <TableHead className="w-[130px]">应用密码</TableHead>
              <TableHead className="w-[142px]">最近取件</TableHead>
              <TableHead className="w-[146px]">状态</TableHead>
              <TableHead className="w-[76px] text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && rows.length === 0 && (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={8} className="py-6">
                  <Loader title="正在加载账号…" subtitle="读取本地数据库" size="sm" className="gap-3 p-0" />
                </TableCell>
              </TableRow>
            )}
            {!loading && rows.length === 0 && (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={8} className="p-0">
                  <EmptyState
                    icon={<KeyRound className="size-6" />}
                    title="暂无账号"
                    hint="通过「导入」粘贴账号列表，或点击「新增账号」手动添加"
                    action={
                      <Button variant="outline" size="sm" onClick={() => setImportOpen(true)}>
                        导入账号
                      </Button>
                    }
                  />
                </TableCell>
              </TableRow>
            )}
            {rows.map((row) => (
              <TableRow key={row.id} data-state={selected.includes(row.id) ? 'selected' : undefined}>
                <TableCell>
                  <Checkbox
                    checked={selected.includes(row.id)}
                    onCheckedChange={() => toggleOne(row.id)}
                    aria-label={`选择 ${row.email}`}
                  />
                </TableCell>
                <TableCell>
                  <div className="flex min-w-0 items-center gap-1">
                    <span className="truncate text-[12px] font-medium">{row.email}</span>
                    <IconAction label="复制邮箱" onClick={() => void copyValue(row.email, '邮箱')}>
                      <Copy className="size-3" />
                    </IconAction>
                  </div>
                  {row.note && (
                    <p className="truncate text-[10px] text-muted-foreground">{row.note}</p>
                  )}
                </TableCell>
                <TableCell>
                  <SecretCell value={row.password} onCopy={() => void copyValue(row.password, '密码')} />
                </TableCell>
                <TableCell>
                  {row.hasTotp ? (
                    <button
                      type="button"
                      onClick={() => {
                        setTotp(null)
                        setTotpFor(row)
                      }}
                      className="inline-flex items-center gap-1 rounded-md px-1 py-0.5 text-[11px] text-primary transition-colors hover:bg-primary/8"
                    >
                      <KeyRound className="size-3" />
                      查看 2FA
                    </button>
                  ) : (
                    <span className="text-[11px] text-muted-foreground/70">未配置</span>
                  )}
                </TableCell>
                <TableCell>
                  <SecretCell
                    value={row.appPassword}
                    onCopy={() => void copyValue(row.appPassword, '应用密码')}
                  />
                </TableCell>
                <TableCell>
                  <p className="text-[11px] whitespace-nowrap">{relativeTime(row.lastFetchAt)}</p>
                  <p className="text-[9.5px] whitespace-nowrap text-muted-foreground/80">
                    {row.lastFetchAt ? formatDateTime(row.lastFetchAt) : '—'}
                  </p>
                </TableCell>
                <TableCell>
                  <FetchStatus account={row} />
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-0.5">
                    <IconAction
                      label="编辑"
                      onClick={() => {
                        setEditing(row)
                        setDialogOpen(true)
                      }}
                    >
                      <Pencil className="size-3.5" />
                    </IconAction>
                    <IconAction label="取件链接" onClick={() => void openLink(row)}>
                      <Link2 className="size-3.5" />
                    </IconAction>
                    <IconAction label="删除" danger onClick={() => setDeleteTarget(row)}>
                      <Trash2 className="size-3.5" />
                    </IconAction>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>

        <div className="flex flex-col items-center gap-2 border-t border-border px-3 py-2 sm:flex-row sm:justify-between">
          <span className="text-[10.5px] text-muted-foreground">
            第 {page} / {pageCount} 页 · 共 {total} 条 · 每页 {PAGE_SIZE} 条
          </span>
          <Pagination page={page} pageCount={pageCount} onChange={setPage} />
        </div>
      </div>

      <AccountDialog
        open={dialogOpen}
        account={editing}
        onClose={() => setDialogOpen(false)}
        onSaved={() => void load({ silent: true })}
      />

      <ImportDialog
        open={importOpen}
        onClose={() => setImportOpen(false)}
        onDone={() => {
          setPage(1)
          void load({ silent: true })
        }}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="删除账号"
        confirmLabel="删除"
        danger
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
      >
        确认删除 <strong>{deleteTarget?.email}</strong>？此操作不可撤销，账号凭据将从数据库中移除。
      </ConfirmDialog>

      <ConfirmDialog
        open={bulkDelete}
        title="批量删除"
        confirmLabel={`删除 ${selected.length} 个`}
        danger
        onClose={() => setBulkDelete(false)}
        onConfirm={handleBulkDelete}
      >
        确认删除选中的 {selected.length} 个账号？此操作不可撤销。
      </ConfirmDialog>

      <TotpDialog account={totpFor} totp={totp} onClose={() => setTotpFor(null)} onCopy={copyValue} />

      <LinkDialog
        account={linkFor}
        busy={linkBusy}
        onClose={() => setLinkFor(null)}
        onCopy={copyValue}
        onRevoked={() => {
          setLinkFor(null)
          toast('链接已撤销', 'success')
        }}
        revoke={async (id) => {
          await api.revokeLink(id)
        }}
      />
    </div>
  )
}

/** Small labelled row used inside the mobile account cards. */
function MobileField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 items-center gap-1">
      <span className="shrink-0 text-muted-foreground/70">{label}</span>
      <span className="min-w-0 truncate">{children}</span>
    </div>
  )
}

function IconAction({
  label,
  onClick,
  children,
  danger,
}: {
  label: string
  onClick: () => void
  children: React.ReactNode
  danger?: boolean
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onClick}
          aria-label={label}
          className={
            danger
              ? 'rounded-md p-1 text-muted-foreground/70 transition-colors hover:bg-destructive/10 hover:text-destructive'
              : 'rounded-md p-1 text-muted-foreground/70 transition-colors hover:bg-accent hover:text-foreground'
          }
        >
          {children}
        </button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function SecretCell({ value, onCopy }: { value: string; onCopy: () => void }) {
  const masked = !value || value.startsWith('••')
  return (
    <div className="flex min-w-0 items-center gap-0.5">
      <span
        className={
          masked
            ? 'truncate text-[11px] text-muted-foreground/60'
            : 'max-w-[110px] truncate font-mono text-[10.5px]'
        }
      >
        {value || '未设置'}
      </span>
      {!masked && (
        <IconAction label="复制" onClick={onCopy}>
          <Copy className="size-2.5" />
        </IconAction>
      )}
    </div>
  )
}

function FetchStatus({ account }: { account: Account }) {
  if (!account.lastFetchAt) {
    return <span className="text-[10.5px] text-muted-foreground/70">未取件</span>
  }
  if (account.lastFetchStatus === 'error') {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="destructive" className="cursor-help">
            取件失败
          </Badge>
        </TooltipTrigger>
        <TooltipContent className="max-w-[260px] text-[11px] leading-relaxed">
          {account.lastFetchError}
        </TooltipContent>
      </Tooltip>
    )
  }
  return (
    <div className="min-w-0">
      <Badge variant="success" className="mb-0.5">
        正常
      </Badge>
      {account.lastFetchSubject && (
        <p className="truncate text-[10px] text-muted-foreground">{account.lastFetchSubject}</p>
      )}
    </div>
  )
}

function ConfirmDialog({
  open,
  title,
  children,
  confirmLabel,
  danger,
  onClose,
  onConfirm,
}: {
  open: boolean
  title: string
  children: React.ReactNode
  confirmLabel: string
  danger?: boolean
  onClose: () => void
  onConfirm: () => void | Promise<void>
}) {
  const [busy, setBusy] = useState(false)
  return (
    <Dialog open={open} onOpenChange={(v) => !v && !busy && onClose()}>
      <DialogContent className="sm:max-w-sm" showCloseButton={!busy}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription asChild>
            <div className="text-[11.5px] leading-relaxed text-muted-foreground">{children}</div>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={onClose} disabled={busy}>
            取消
          </Button>
          <Button
            size="sm"
            variant={danger ? 'destructive' : 'default'}
            disabled={busy}
            onClick={async () => {
              setBusy(true)
              try {
                await onConfirm()
              } finally {
                setBusy(false)
              }
            }}
          >
            {busy && <Loader2 className="size-3.5 animate-spin" />}
            {busy ? '处理中…' : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function TotpDialog({
  account,
  totp,
  onClose,
  onCopy,
}: {
  account: Account | null
  totp: { code: string; remaining: number } | null
  onClose: () => void
  onCopy: (value: string, label: string) => void
}) {
  return (
    <Dialog open={Boolean(account)} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>2FA 验证码</DialogTitle>
          <DialogDescription className="truncate">{account?.email}</DialogDescription>
        </DialogHeader>
        <div className="px-4 py-4">
          <div className="flex items-center gap-2.5">
            <AnimatePresence mode="popLayout" initial={false}>
              <motion.span
                key={totp?.code ?? 'empty'}
                initial={{ opacity: 0, y: -6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: 6 }}
                transition={{ duration: 0.16 }}
                className="font-mono text-[26px] leading-none font-semibold tracking-[0.12em] tabular-nums"
              >
                {totp ? `${totp.code.slice(0, 3)} ${totp.code.slice(3)}` : '—— ——'}
              </motion.span>
            </AnimatePresence>
            <Badge variant="outline">{totp ? `${totp.remaining}s` : '—'}</Badge>
            <IconAction label="复制验证码" onClick={() => totp && onCopy(totp.code, '验证码')}>
              <Copy className="size-3.5" />
            </IconAction>
          </div>
          <p className="mt-3 text-[10.5px] leading-relaxed text-muted-foreground">
            基于 TOTP (SHA1 / 30 秒 / 6 位) 实时计算，每 5 秒自动刷新。
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ImportDialog({
  open,
  onClose,
  onDone,
}: {
  open: boolean
  onClose: () => void
  onDone: () => void
}) {
  const { toast } = useToast()
  const [content, setContent] = useState('')
  const [overwrite, setOverwrite] = useState(false)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [error, setError] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setContent('')
      setResult(null)
      setError('')
      setOverwrite(false)
    }
  }, [open])

  const lineCount = useMemo(
    () => content.split('\n').filter((l) => l.trim() && !l.trim().startsWith('#')).length,
    [content],
  )

  const pickFile = async (file?: File) => {
    if (!file) return
    if (file.size > 8 * 1024 * 1024) {
      toast('文件过大（上限 8MB）', 'error')
      return
    }
    setContent(await file.text())
  }

  const submit = async () => {
    setBusy(true)
    setError('')
    try {
      const res = await api.importAccounts(content, overwrite)
      setResult(res)
      toast(
        `导入完成：新增 ${res.created}，更新 ${res.updated}，跳过 ${res.skipped}，失败 ${res.failed}`,
        res.failed ? 'warning' : 'success',
      )
      onDone()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '导入失败')
    } finally {
      setBusy(false)
    }
  }

  const failures = result?.rows.filter((r) => r.status === 'failed') ?? []

  return (
    <Dialog open={open} onOpenChange={(v) => !v && !busy && onClose()}>
      <DialogContent className="sm:max-w-xl" showCloseButton={!busy}>
        <DialogHeader>
          <DialogTitle>导入账号</DialogTitle>
          <DialogDescription>每行一个账号，支持粘贴或选择文件</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-3">
          <div className="rounded-lg border border-border bg-muted/40 px-3 py-2 text-[10.5px] leading-relaxed text-muted-foreground">
            每行格式：<code className="rounded bg-background px-1 py-px font-mono">邮箱----密码----2FA密钥----应用专用密码</code>
            <br />
            也支持 Tab / 竖线 / 逗号分隔；空行与 # 开头的行会被忽略。
          </div>

          <div className="flex flex-wrap items-center gap-2.5">
            <Button variant="outline" size="xs" onClick={() => fileRef.current?.click()}>
              <Upload className="size-3" />
              选择文件
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept=".txt,.csv,text/plain"
              hidden
              onChange={(e) => {
                void pickFile(e.target.files?.[0])
                e.target.value = ''
              }}
            />
            <div className="flex items-center gap-1.5">
              <Switch id="overwrite" checked={overwrite} onCheckedChange={setOverwrite} />
              <Label htmlFor="overwrite" className="text-[11px] font-normal text-muted-foreground">
                覆盖已存在账号
              </Label>
            </div>
            <div className="flex-1" />
            <span className="text-[10.5px] text-muted-foreground">待导入 {lineCount} 行</span>
          </div>

          <Textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            rows={9}
            placeholder={
              'user@gmail.com----accountpass----JBSWY3DPEHPK3PXP----abcdefghijklmnop\nuser2@gmail.com----pass2--------abcdefghijklmnop'
            }
            className="max-h-64 min-h-36 font-mono text-[11px]"
          />

          {error && (
            <div className="rounded-md bg-destructive/8 px-2.5 py-1.5 text-[11.5px] text-destructive">
              {error}
            </div>
          )}

          {result && (
            <div className="overflow-hidden rounded-lg border border-border">
              <div className="flex flex-wrap gap-4 px-3 py-2">
                <Stat label="新增" value={result.created} tone="text-emerald-600 dark:text-emerald-400" />
                <Stat label="更新" value={result.updated} tone="text-primary" />
                <Stat label="跳过" value={result.skipped} tone="text-amber-600 dark:text-amber-400" />
                <Stat label="失败" value={result.failed} tone="text-destructive" />
              </div>
              {failures.length > 0 && (
                <>
                  <Separator />
                  <div className="max-h-36 overflow-auto">
                    {failures.map((f, i) => (
                      <div key={`${f.email}-${i}`} className="border-b border-border px-3 py-1.5 last:border-0">
                        <p className="font-mono text-[11px]">{f.email}</p>
                        <p className="text-[10px] text-destructive">{f.message}</p>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={onClose} disabled={busy}>
            {result ? '关闭' : '取消'}
          </Button>
          <Button size="sm" onClick={submit} disabled={busy || lineCount === 0}>
            {busy && <Loader2 className="size-3.5 animate-spin" />}
            {busy ? '导入中…' : '开始导入'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Shows a single account's public fetch link with copy / open / revoke. */
function LinkDialog({
  account,
  busy,
  onClose,
  onCopy,
  onRevoked,
  revoke,
}: {
  account: (Account & { shareUrl?: string }) | null
  busy: boolean
  onClose: () => void
  onCopy: (value: string, label: string) => void
  onRevoked: () => void
  revoke: (id: number) => Promise<void>
}) {
  const [confirmRevoke, setConfirmRevoke] = useState(false)
  const [revoking, setRevoking] = useState(false)
  const url = account?.shareUrl ?? ''

  useEffect(() => {
    if (!account) {
      setConfirmRevoke(false)
      setRevoking(false)
    }
  }, [account])

  return (
    <Dialog open={Boolean(account)} onOpenChange={(v) => !v && !revoking && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>取件链接</DialogTitle>
          <DialogDescription className="truncate">{account?.email}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3 px-4 py-3">
          <p className="text-[11px] leading-relaxed text-muted-foreground">
            任何人持有该链接即可在无需登录的情况下查看此账号的最新 10 封邮件（含垃圾箱）。请谨慎分享。
          </p>

          <div className="rounded-lg border border-border bg-muted/40 p-2">
            <p className="mb-1.5 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
              链接
            </p>
            {busy ? (
              <div className="flex items-center gap-2 py-1 text-[11.5px] text-muted-foreground">
                <Loader2 className="size-3.5 animate-spin" />
                正在生成…
              </div>
            ) : (
              <p className="font-mono text-[10.5px] leading-relaxed break-all select-all">{url}</p>
            )}
          </div>

          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="xs"
              disabled={!url}
              onClick={() => onCopy(url, '取件链接')}
            >
              <Copy className="size-3" />
              复制链接
            </Button>
            <Button variant="outline" size="xs" disabled={!url} asChild>
              <a href={url || '#'} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="size-3" />
                打开
              </a>
            </Button>
            {confirmRevoke ? (
              <Button
                size="xs"
                variant="destructive"
                disabled={revoking}
                onClick={async () => {
                  if (!account) return
                  setRevoking(true)
                  try {
                    await revoke(account.id)
                    onRevoked()
                  } finally {
                    setRevoking(false)
                  }
                }}
              >
                {revoking ? <Loader2 className="size-3 animate-spin" /> : <Trash2 className="size-3" />}
                确认撤销
              </Button>
            ) : (
              <Button size="xs" variant="ghost" onClick={() => setConfirmRevoke(true)}>
                撤销链接
              </Button>
            )}
          </div>

          <p className="text-[10px] leading-relaxed text-muted-foreground/80">
            撤销后旧链接立即失效，再次点击链接图标会生成新的链接。
          </p>
        </div>

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={onClose} disabled={revoking}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Stat({ label, value, tone }: { label: string; value: number; tone: string }) {
  return (
    <div className="flex items-baseline gap-1">
      <span className="text-[10.5px] text-muted-foreground">{label}</span>
      <span className={`text-[12.5px] font-semibold ${tone}`}>{value}</span>
    </div>
  )
}

/** Toggle used inside the account form. */
export function SecretToggle({ shown, onToggle }: { shown: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="rounded p-1 text-muted-foreground/70 transition-colors hover:text-foreground"
      aria-label={shown ? '隐藏' : '显示'}
    >
      {shown ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
    </button>
  )
}

/** Small check mark used to confirm a successful copy. */
export function CopyCheck() {
  return <Check className="size-3" />
}
