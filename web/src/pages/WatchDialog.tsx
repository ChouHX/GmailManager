import { useEffect, useMemo, useState } from 'react'
import { Loader2, Save, Send, Webhook } from 'lucide-react'
import { api, ApiError, type Account, type Watch } from '../api'
import { useToast } from '../components/ToastProvider'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const FOLDERS = [
  { value: 'inbox', label: '仅收件箱' },
  { value: 'auto', label: '收件箱 + 垃圾箱' },
  { value: 'spam', label: '仅垃圾箱' },
  { value: 'trash', label: '仅已删除' },
]

const INTERVAL_PRESETS = [
  { label: '1 分钟', value: 60 },
  { label: '5 分钟', value: 300 },
  { label: '15 分钟', value: 900 },
  { label: '30 分钟', value: 1800 },
  { label: '1 小时', value: 3600 },
  { label: '6 小时', value: 21600 },
]

interface FormState {
  accountId: string
  name: string
  subject: string
  from: string
  folder: string
  intervalSeconds: string
  webhookUrl: string
  enabled: boolean
}

const EMPTY: FormState = {
  accountId: '',
  name: '',
  subject: '',
  from: '',
  folder: 'inbox',
  intervalSeconds: '300',
  webhookUrl: '',
  enabled: true,
}

interface WatchDialogProps {
  open: boolean
  watch: Watch | null
  onClose: () => void
  onSaved: () => void
}

export function WatchDialog({ open, watch, onClose, onSaved }: WatchDialogProps) {
  const { toast } = useToast()
  const [form, setForm] = useState<FormState>(EMPTY)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [busy, setBusy] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState<{ tone: 'ok' | 'bad'; text: string } | null>(null)

  const editing = Boolean(watch)

  useEffect(() => {
    if (!open) return
    setError('')
    setNotice(null)
    api
      .listAccounts(1, 200, '')
      .then((res) => setAccounts(res.items ?? []))
      .catch(() => setAccounts([]))
    if (watch) {
      setForm({
        accountId: String(watch.accountId),
        name: watch.name,
        subject: watch.subject,
        from: watch.from,
        folder: watch.folder || 'inbox',
        intervalSeconds: String(watch.intervalSeconds),
        webhookUrl: watch.webhookUrl,
        enabled: watch.enabled,
      })
    } else {
      setForm(EMPTY)
    }
  }, [open, watch])

  const interval = Number(form.intervalSeconds)
  const intervalValid = Number.isFinite(interval) && interval >= 30 && interval <= 86400
  const hasCondition = Boolean(form.subject.trim() || form.from.trim())
  const webhookValid = /^https?:\/\/.+\/api\/webhooks\//i.test(form.webhookUrl.trim())
  const canSave =
    Boolean(form.accountId) && hasCondition && intervalValid && webhookValid && !busy

  const set = (key: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [key]: e.target.value }))

  const payload = useMemo(
    () => ({
      accountId: Number(form.accountId) || 0,
      name: form.name.trim(),
      subject: form.subject.trim(),
      from: form.from.trim(),
      folder: form.folder,
      intervalSeconds: Number(form.intervalSeconds) || 0,
      webhookUrl: form.webhookUrl.trim(),
      enabled: form.enabled,
    }),
    [form],
  )

  const submit = async () => {
    setBusy(true)
    setError('')
    try {
      if (watch) {
        await api.updateWatch(watch.id, payload)
        toast('任务已更新', 'success')
      } else {
        await api.createWatch(payload)
        toast('任务已创建', 'success')
      }
      onSaved()
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  const testHook = async () => {
    if (!webhookValid) {
      setNotice({ tone: 'bad', text: '请先填写有效的 Discord webhook 链接' })
      return
    }
    setTesting(true)
    setNotice(null)
    try {
      const res = await api.testWebhook(payload.webhookUrl)
      setNotice({ tone: res.ok ? 'ok' : 'bad', text: res.message })
    } catch (err) {
      setNotice({ tone: 'bad', text: err instanceof ApiError ? err.message : '测试失败' })
    } finally {
      setTesting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && !busy && onClose()}>
      <DialogContent className="sm:max-w-lg" showCloseButton={!busy}>
        <DialogHeader>
          <DialogTitle>{editing ? '编辑轮询任务' : '新建轮询任务'}</DialogTitle>
          <DialogDescription>
            定时轮询该账号最近 5 封邮件，标题与发件人同时命中时推送 Discord 通知
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-3">
          <div className="space-y-1">
            <Label className="text-[11px] text-muted-foreground">轮询账号</Label>
            <Select
              value={form.accountId}
              onValueChange={(v) => setForm((prev) => ({ ...prev, accountId: v }))}
              disabled={busy}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder="选择要轮询的邮箱" />
              </SelectTrigger>
              <SelectContent>
                {accounts.length === 0 && (
                  <SelectItem value="__none" disabled>
                    暂无账号，请先在账号管理中导入
                  </SelectItem>
                )}
                {accounts.map((a) => (
                  <SelectItem key={a.id} value={String(a.id)} className="text-[12px]">
                    {a.email}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1">
            <Label htmlFor="watch-name" className="text-[11px] text-muted-foreground">
              任务名称（可选，用于通知标题）
            </Label>
            <Input
              id="watch-name"
              value={form.name}
              onChange={set('name')}
              disabled={busy}
              placeholder="例如：银行验证码"
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="watch-subject" className="text-[11px] text-muted-foreground">
                邮件标题条件
              </Label>
              <Input
                id="watch-subject"
                value={form.subject}
                onChange={set('subject')}
                disabled={busy}
                placeholder="如 验证码 或 *verify*code*"
                className="font-mono text-[11.5px]"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="watch-from" className="text-[11px] text-muted-foreground">
                发件人条件
              </Label>
              <Input
                id="watch-from"
                value={form.from}
                onChange={set('from')}
                disabled={busy}
                placeholder="如 @bank.com"
                className="font-mono text-[11.5px]"
              />
            </div>
          </div>
          <p className="text-[10px] leading-relaxed text-muted-foreground/85">
            标题与发件人可只填一个；纯文本为包含匹配，<code className="font-mono">*</code> 为通配，
            <code className="font-mono">re:正则</code> 为正则，<code className="font-mono">case:</code>{' '}
            前缀区分大小写。
          </p>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label className="text-[11px] text-muted-foreground">取件范围</Label>
              <Select
                value={form.folder}
                onValueChange={(v) => setForm((prev) => ({ ...prev, folder: v }))}
                disabled={busy}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {FOLDERS.map((f) => (
                    <SelectItem key={f.value} value={f.value} className="text-[12px]">
                      {f.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="watch-interval" className="text-[11px] text-muted-foreground">
                轮询间隔（秒）
              </Label>
              <Input
                id="watch-interval"
                type="number"
                min={30}
                max={86400}
                value={form.intervalSeconds}
                onChange={set('intervalSeconds')}
                disabled={busy}
                aria-invalid={Boolean(form.intervalSeconds) && !intervalValid}
                className="font-mono text-[11.5px]"
              />
            </div>
          </div>
          <div className="flex flex-wrap gap-1">
            {INTERVAL_PRESETS.map((p) => (
              <Button
                key={p.value}
                type="button"
                variant={Number(form.intervalSeconds) === p.value ? 'secondary' : 'outline'}
                size="xs"
                disabled={busy}
                onClick={() => setForm((prev) => ({ ...prev, intervalSeconds: String(p.value) }))}
              >
                {p.label}
              </Button>
            ))}
          </div>

          <div className="space-y-1">
            <Label htmlFor="watch-hook" className="text-[11px] text-muted-foreground">
              Discord Webhook 链接
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id="watch-hook"
                value={form.webhookUrl}
                onChange={set('webhookUrl')}
                disabled={busy}
                placeholder="https://discord.com/api/webhooks/…"
                className="font-mono text-[11px]"
                aria-invalid={Boolean(form.webhookUrl) && !webhookValid}
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void testHook()}
                disabled={busy || testing}
              >
                {testing ? <Loader2 className="size-3 animate-spin" /> : <Send className="size-3" />}
                测试
              </Button>
            </div>
            <p className="text-[10px] text-muted-foreground/85">
              Discord 频道设置 → 整合 → Webhook → 复制 Webhook 网址。
            </p>
          </div>

          <div className="flex items-center justify-between rounded-lg border border-border bg-muted/25 px-3 py-2">
            <div className="min-w-0">
              <p className="text-[11.5px] font-medium">启用定时轮询</p>
              <p className="text-[10px] text-muted-foreground">
                停用后保留配置，但不再自动轮询
              </p>
            </div>
            <Switch
              checked={form.enabled}
              onCheckedChange={(v) => setForm((prev) => ({ ...prev, enabled: v }))}
              disabled={busy}
              aria-label="启用定时轮询"
            />
          </div>

          {error && (
            <div className="rounded-md bg-destructive/8 px-2.5 py-1.5 text-[11.5px] text-destructive">
              {error}
            </div>
          )}
          {notice && (
            <div
              className={
                notice.tone === 'ok'
                  ? 'rounded-md bg-emerald-500/10 px-2.5 py-1.5 text-[11.5px] text-emerald-700 dark:text-emerald-400'
                  : 'rounded-md bg-destructive/8 px-2.5 py-1.5 text-[11.5px] text-destructive'
              }
            >
              {notice.text}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={onClose} disabled={busy}>
            取消
          </Button>
          <Button size="sm" onClick={submit} disabled={!canSave}>
            {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
            {busy ? '保存中…' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Exported so the page header can reuse the same icon. */
export const WatchIcon = Webhook
